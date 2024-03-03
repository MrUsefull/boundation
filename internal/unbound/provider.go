package unbound

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"github.com/MrUsefull/boundation/internal/config"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

const (
	// apiPrefix is the root path for API operations on unbound in opnsense.
	apiPrefix = "/api/unbound"
	// SearchOverridesEndpoint is used to get existing DNS entries in unbound.
	SearchOverridesEndpoint = apiPrefix + "/settings/searchHostOverride"
	// AddOverrideEndpoint is the create DNS entries.
	AddOverrideEndpoint = apiPrefix + "/settings/addHostOverride"
	// DelOverrideEndpoint is the api endpoint for deleting DNS entries.
	DelOverrideEndpoint = apiPrefix + "/settings/delHostOverride/"

	ApplyChangesEndpoint = apiPrefix + "/service/reconfigure"

	authHeader = "Authorization"

	CreateOpSuccessResponse = "saved"
	DeleteOpSuccessResponse = "deleted"

	DescriptionPrefix = "Managed by K8s external-dns"
)

var (
	ErrRequestFailed = errors.New("request failed")
	ErrMarshalling   = errors.New("marshal response")
)

var _ provider.Provider = &Unbound{}

// OperationResponse is the in memory representation of
// success/fail response from opnsense unbound api
// success response is typically the opSuccessResponse value.
type OperationResponse struct {
	Result string `json:"result"`
}

// Unbound is the opnsense unbound dns provider implementation.
// opnsense unbound does not support txt records. Txt records will be added
// to the "description" field of a record.
type Unbound struct {
	client  *http.Client
	baseURL string
	creds   string
	logger  *slog.Logger

	domainFilter endpoint.DomainFilter

	// knownRecords tracks dns records we've seen and their associated "TXT Record" - ie description
	// unbound does not support txt records, so we stuff txt records into the description field
	knownRecords *cache
}

// New creates an Unbound provider
// client - the http client to use
// baseUrl - location of the opnsense unbound API
// creds - credentials in the form of apiKey:apiSecret.
func New(client *http.Client, cfg config.Config, logger *slog.Logger) *Unbound {
	return &Unbound{
		client:       client,
		baseURL:      cfg.BaseURL,
		creds:        basicAuthEncoding(cfg.Creds),
		domainFilter: endpoint.NewDomainFilterWithExclusions(cfg.Filter, cfg.Exclude),
		logger:       logger,
		knownRecords: newCache(logger),
	}
}

// Records returns all records or "overrides" in opnsense unbound. Unbound does not support
// txt record types. If a record is managed by external-dns, it will have the associated txt records
// in the description field. Records will marshall the txt fields into a separate endpoint.
func (u Unbound) Records(ctx context.Context) ([]*endpoint.Endpoint, error) {
	url := u.baseURL + SearchOverridesEndpoint
	u.logger.InfoContext(ctx, "records request url", slog.String("URL", url))

	req, err := u.apiRequest(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("records request: %w", err)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("records response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %v: %w", resp.Status, ErrRequestFailed)
	}

	endpoints, err := u.marshalEndpoints(ctx, resp)
	if err != nil {
		return nil, errors.Join(ErrMarshalling, err)
	}

	u.knownRecords.updateReadRecords(endpoints)

	return endpoints, nil
}

func (u Unbound) ApplyChanges(ctx context.Context, changes *plan.Changes) error {
	if !changes.HasChanges() {
		u.logger.DebugContext(ctx, "no changes to apply")

		return nil
	}

	u.knownRecords.updateFromPlan(changes)

	if err := u.deleteEndpoints(ctx, append(changes.Delete, changes.UpdateOld...)); err != nil {
		return fmt.Errorf("plan delete: %w", err)
	}

	if err := u.createEndpoints(ctx, append(changes.Create, changes.UpdateNew...)); err != nil {
		return fmt.Errorf("plan create: %w", err)
	}

	if err := u.reconfigure(ctx); err != nil {
		return fmt.Errorf("apply changes reconfigure endpoint: %w", err)
	}

	return nil
}

// AdjustEndpoints canonicalizes a set of candidate endpoints.
// It is called with a set of candidate endpoints obtained from the various sources.
// It returns a set modified as required by the provider. The provider is responsible for
// adding, removing, and modifying the ProviderSpecific properties to match
// the endpoints that the provider returns in `Records` so that the change plan will not have
// unnecessary (potentially failing) changes. It may also modify other fields, add, or remove
// Endpoints. It is permitted to modify the supplied endpoints.
func (u Unbound) AdjustEndpoints(endpoints []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return endpoints, nil
}

func (u Unbound) GetDomainFilter() endpoint.DomainFilter {
	return u.domainFilter
}

func (u Unbound) createEndpoints(ctx context.Context, endpoints []*endpoint.Endpoint) error {
	for _, ep := range endpoints {
		if supportedType(ep.RecordType) {
			if err := u.createEndpoint(ctx, ep); err != nil {
				return fmt.Errorf("create endpoint: %w", err)
			}
		} else {
			slog.DebugContext(ctx, "skipping create, wrong record type",
				slog.String("type", ep.RecordType),
				slog.Any("endpoint", ep))
		}
	}

	return nil
}

func (u Unbound) createEndpoint(ctx context.Context, endpoint *endpoint.Endpoint) error {
	requestDatas, err := u.createRequestJSON(endpoint)
	if err != nil {
		return fmt.Errorf("create endpoint: %w", err)
	}

	for _, data := range requestDatas {
		if err := u.createTarget(ctx, data, endpoint); err != nil {
			return err
		}
	}

	return nil
}

func (u Unbound) createTarget(ctx context.Context, data []byte, endpoint *endpoint.Endpoint) error {
	u.logger.InfoContext(ctx, "creating endpoint", slog.String("data", string(data)))

	req, err := u.postAPIRequest(ctx,
		u.baseURL+AddOverrideEndpoint,
		data)
	if err != nil {
		return fmt.Errorf("create endpoint request: %w", err)
	}

	resp, err := u.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return fmt.Errorf("create http do: %w", err)
	}

	body := u.responseBody(ctx, AddOverrideEndpoint, resp.Body)

	if resp.StatusCode != http.StatusOK {
		u.logger.InfoContext(ctx, "response status not OK",
			slog.Any("status", resp.Status),
			slog.String("endpoint", endpoint.DNSName),
		)

		return fmt.Errorf("response status: %v: %w", resp.StatusCode, ErrRequestFailed)
	}

	return u.checkResponse(ctx, body, CreateOpSuccessResponse)
}

func (u Unbound) deleteEndpoints(ctx context.Context, endpoints []*endpoint.Endpoint) error {
	for _, endpoint := range endpoints {
		if err := u.deleteEndpoint(ctx, endpoint); err != nil {
			return fmt.Errorf("%q: %w", endpoint.DNSName, err)
		}
	}

	return nil
}

func (u Unbound) deleteEndpoint(ctx context.Context, endpoint *endpoint.Endpoint) error {
	urlPath := path.Join(DelOverrideEndpoint, endpoint.SetIdentifier)
	url := u.baseURL + urlPath
	u.logger.InfoContext(ctx, "delete endpoint request", slog.String("url", url))

	reqBody := emptyJSON()

	req, err := u.apiRequest(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}

	u.logger.InfoContext(ctx, "making delete request", slog.String("url", req.URL.String()))

	resp, err := u.client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return fmt.Errorf("delete http do: %w", err)
	}

	body := u.responseBody(ctx, DelOverrideEndpoint, resp.Body)

	if resp.StatusCode != http.StatusOK {
		u.logger.InfoContext(ctx, "delete response status not OK",
			slog.Any("status", resp.Status),
			slog.String("endpoint", endpoint.DNSName))

		return fmt.Errorf("response status: %v: %w", resp.StatusCode, ErrRequestFailed)
	}

	return u.checkResponse(ctx, body, DeleteOpSuccessResponse)
}

// reconfigure calls the same endpoint as the "apply" button in the UI.
func (u Unbound) reconfigure(ctx context.Context) error {
	url := u.baseURL + ApplyChangesEndpoint
	req, err := u.postAPIRequest(ctx, url, emptyJSON())
	if err != nil {
		return fmt.Errorf("reconfigure request create: %w", err)
	}
	resp, err := u.client.Do(req)
	if resp != nil && resp.Body != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return fmt.Errorf("reconfigure: %w", err)
	}

	body := u.responseBody(ctx, ApplyChangesEndpoint, resp.Body)

	if resp.StatusCode != http.StatusOK {
		u.logger.ErrorContext(ctx, "failed to apply reconfigure",
			slog.Any("response status", resp.StatusCode),
			slog.String("body", string(body)),
		)
		return fmt.Errorf("response status: %v: %w", resp.StatusCode, ErrRequestFailed)
	}
	return u.checkResponse(ctx, body, "")
}

func (u Unbound) marshalEndpoints(ctx context.Context, resp *http.Response) ([]*endpoint.Endpoint, error) {
	body := resp.Body
	defer body.Close()

	buff, err := io.ReadAll(body)
	if err != nil {
		return nil, fmt.Errorf("body read err: %w", err)
	}

	u.logger.DebugContext(ctx,
		"received response from opnsense",
		slog.String("status", resp.Status),
		slog.String("body", string(buff)))

	searchResp := &SearchHostResp{}
	if err := json.Unmarshal(buff, searchResp); err != nil {
		return nil, fmt.Errorf("unmarshal resp: %w", err)
	}

	u.logger.DebugContext(ctx, "unmarshalled successfully", slog.Any("searchResp", searchResp))

	return searchResp.ToEndpoints(), nil
}

func basicAuthEncoding(creds string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(creds))
}

func (u Unbound) createRequestJSON(endpoint *endpoint.Endpoint) ([][]byte, error) {
	out := make([][]byte, 0, len(endpoint.Targets))
	dnsSplit := strings.Split(endpoint.DNSName, ".")
	for _, target := range endpoint.Targets {
		record := Record{
			Enabled:     "1",
			Hostname:    dnsSplit[0],
			Domain:      strings.Join(dnsSplit[1:], "."),
			Server:      target,
			Rr:          endpoint.RecordType,
			Description: u.knownRecords.createDescription(endpoint.DNSName),
		}
		request := &AddOverrideRequest{
			Host: record,
		}

		jsonOut, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("marshal: %w", err)
		}
		out = append(out, jsonOut)
	}

	return out, nil
}

func logResponse(ctx context.Context, logger *slog.Logger, requestEndpoint string, body []byte) {
	logger.InfoContext(ctx, "response", slog.String("request", requestEndpoint), slog.String("body", string(body)))
}

func (u Unbound) postAPIRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	req, err := u.apiRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/json")

	return req, nil
}

func (u Unbound) apiRequest(ctx context.Context, method string, path string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx,
		method,
		path,
		bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Add(authHeader, u.creds)

	return req, nil
}

func (u Unbound) responseBody(ctx context.Context, requestID string, reader io.Reader) []byte {
	body, err := io.ReadAll(reader)
	if err != nil {
		u.logger.InfoContext(ctx, "failed to read response body", slog.Any("error", err))
	}

	logResponse(ctx, u.logger, requestID, body)

	return body
}

func (u Unbound) checkResponse(ctx context.Context, body []byte, wantResult string) error {
	result := &OperationResponse{}
	if err := json.Unmarshal(body, result); err != nil || result.Result != wantResult {
		u.logger.WarnContext(ctx, "operation was not a success", slog.Any("error", err), slog.Any("response", result.Result))

		return fmt.Errorf("response %q: %w", result.Result, ErrRequestFailed)
	}

	return nil
}

func emptyJSON() []byte {
	return []byte(`"{}"`)
}
