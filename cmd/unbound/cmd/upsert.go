package cmd

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/spf13/cobra"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

var (
	ErrUnequalHostTargets = errors.New("the hosts and targets must have the same length")

	exampleUpsert = fmt.Sprintf(
		"upsert --%v=host1.example.com --%v=host2.example.fqdn --%v=10.11.12.13 --%v=10.0.0.3",
		hostsFlag,
		targetsFlag,
		hostsFlag,
		targetsFlag,
	)
)

var upsertCMD = &cobra.Command{
	Use:     exampleUpsert,
	Short:   "Upserts the provided dns overrides in OPNsense unbound",
	Example: exampleUpsert,
	RunE:    configured(runUpsert),
}

func runUpsert(cmd *cobra.Command, _ []string) error {
	creator := newUpsert(http.DefaultClient, pkgConfig, logger)
	return creator.doUpsert(cmd)
}

func parseHostMappings(cmd *cobra.Command) (map[string]string, error) {
	hosts, err := cmd.Flags().GetStringArray(hostsFlag)
	if err != nil {
		return nil, fmt.Errorf("missing hosts: %w", err)
	}
	targets, err := cmd.Flags().GetStringArray(targetsFlag)
	if err != nil {
		return nil, fmt.Errorf("missing targets: %w", err)
	}
	return toMapping(hosts, targets)
}

func toMapping(hosts []string, targets []string) (map[string]string, error) {
	if len(hosts) != len(targets) {
		return nil, ErrUnequalHostTargets
	}
	out := make(map[string]string, len(hosts))
	for i, host := range hosts {
		out[host] = targets[i]
	}
	return out, nil
}

type upsert struct {
	logger   *slog.Logger
	provider *unbound.Unbound
}

func newUpsert(client *http.Client, cfg config.Config, logger *slog.Logger) *upsert {
	return &upsert{
		logger:   logger,
		provider: unbound.New(client, cfg, logger),
	}
}

func (c *upsert) doUpsert(cmd *cobra.Command) error {
	ctx := cmd.Context()
	hostMappings, err := parseHostMappings(cmd)
	if err != nil {
		return err
	}

	existing, err := c.provider.Records(ctx)
	if err != nil {
		return fmt.Errorf("unable to read existing records: %w", err)
	}

	logger.Debug("Starting Create Processing")
	changes := c.createChangeSet(existing, hostMappings)
	if err := c.provider.ApplyChanges(ctx, changes); err != nil {
		return fmt.Errorf("apply failed: %w", err)
	}
	return nil
}

func (c *upsert) createChangeSet(existing []*endpoint.Endpoint, hostMappings map[string]string) *plan.Changes {
	out := &plan.Changes{}
	for _, ep := range existing {
		if val, ok := hostMappings[ep.DNSName]; ok && val == ep.Targets[0] {
			delete(hostMappings, ep.DNSName)
		} else if ok && val != ep.Targets[0] {
			out.Delete = append(out.Delete, ep)
		}
	}
	out.Create = toEndpoints(hostMappings)
	c.logger.Info("create plan created", slog.Any("plan", out))
	return out
}

func toEndpoints(in map[string]string) []*endpoint.Endpoint {
	out := make([]*endpoint.Endpoint, 0, len(in))
	for host, target := range in {
		out = append(out, endpoint.NewEndpoint(host, endpoint.RecordTypeA, target))
	}
	return out
}

func setCreateCmdFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray(hostsFlag, []string{}, "FQDN for DNS entries to add")
	cmd.Flags().StringArray(targetsFlag, []string{}, "ip addresses mapping to hosts")
}
