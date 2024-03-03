package unbound

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
)

type SearchHostResp struct {
	Rows []Record `json:"rows"`
}

func (shr SearchHostResp) ToEndpoints() []*endpoint.Endpoint {
	out := make([]*endpoint.Endpoint, 0, len(shr.Rows))

	for _, row := range shr.Rows {
		marshalledEndpoint := &endpoint.Endpoint{
			DNSName:       fmt.Sprintf("%s.%s", row.Hostname, row.Domain),
			Targets:       endpoint.NewTargets(row.Server),
			RecordType:    strings.Split(row.Rr, " ")[0], // likely bug here
			SetIdentifier: row.UUID,
		}
		if row.Description != "" {
			marshalledEndpoint.Labels = map[string]string{
				"description": row.Description,
			}
		}
		out = append(out, marshalledEndpoint)
		if txtEndpoints, ok := endpointsFromBase64Description(marshalledEndpoint.DNSName, row.Description); ok {
			out = append(out, txtEndpoints...)
		}
	}

	return out
}

func endpointsFromBase64Description(dnsName string, description string) ([]*endpoint.Endpoint, bool) {
	if !strings.HasPrefix(description, DescriptionPrefix) {
		return nil, false
	}
	stripped := strings.TrimSpace(strings.Replace(description, DescriptionPrefix, "", 1))
	decoded, err := base64.StdEncoding.DecodeString(stripped)
	if err != nil {
		slog.Error("unable to base64 decode txt records", slog.Any("error", err))
		return nil, false
	}
	foundTxtRecords := []*endpoint.Endpoint{
		{
			DNSName:    dnsName,
			Targets:    endpoint.NewTargets(string(decoded)),
			RecordType: endpoint.RecordTypeTXT,
		},
		{
			DNSName:    "a-" + dnsName,
			Targets:    endpoint.NewTargets(string(decoded)),
			RecordType: endpoint.RecordTypeTXT,
		},
	}
	return foundTxtRecords, true
}

type Record struct {
	UUID        string `json:"uuid,omitempty"`
	Hostname    string `json:"hostname"`
	Domain      string `json:"domain"`
	Rr          string `json:"rr,omitempty"`
	Server      string `json:"server"`
	Enabled     string `json:"enabled"`
	Description string `json:"description"`
}

func (r Record) DNSName() string {
	return fmt.Sprintf("%v.%v", r.Hostname, r.Domain)
}

type AddOverrideRequest struct {
	Host Record `json:"host"`
}

func supportedType(recordType string) bool {
	switch recordType {
	case endpoint.RecordTypeA, endpoint.RecordTypeAAAA:
		return true
	default:
		return false
	}
}
