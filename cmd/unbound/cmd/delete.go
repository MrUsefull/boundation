package cmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/spf13/cobra"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	"sigs.k8s.io/external-dns/provider"
)

var ErrMissingHosts = errors.New("1 host or more is required")

var exampleDelete = fmt.Sprintf("delete --%v=hostname.example.com --%v=host2.example.com", hostsFlag, hostsFlag)

var deleteCMD = &cobra.Command{
	Use:     exampleDelete,
	Short:   "deletes the provided dns overrides in OPNsense unbound",
	Example: exampleDelete,
	RunE:    configured(runDelete),
}

func runDelete(cmd *cobra.Command, _ []string) error {
	return deleteEndpoints(http.DefaultClient, pkgConfig, cmd)
}

func deleteEndpoints(client *http.Client, cfg config.Config, cmd *cobra.Command) error {
	ctx := cmd.Context()
	toDeleteEP, err := parseDeleteFlags(cmd)
	if err != nil {
		return err
	}
	provider := unbound.New(client, cfg, logger)
	changes, err := planChanges(ctx, provider, toDeleteEP)
	if err != nil {
		return err
	}
	if len(changes.Delete) == 0 {
		fmt.Printf("No changes needed for delete\n")
		return nil
	}
	if err := provider.ApplyChanges(ctx, changes); err != nil {
		return fmt.Errorf("apply changes: %w", err)
	}

	return nil
}

func parseDeleteFlags(cmd *cobra.Command) (map[string]struct{}, error) {
	hosts, err := cmd.Flags().GetStringArray(hostsFlag)
	if err != nil {
		return nil, fmt.Errorf("missing hosts: %w", err)
	}
	if len(hosts) == 0 {
		return nil, ErrMissingHosts
	}
	logger.Info("Want to delete", slog.Any("Hosts", hosts))

	return toSet(hosts), nil
}

func toSet(hosts []string) map[string]struct{} {
	out := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		out[host] = struct{}{}
	}
	return out
}

func planChanges(ctx context.Context, unbonud provider.Provider, toDelete map[string]struct{}) (*plan.Changes, error) {
	found, err := unbonud.Records(ctx)
	if err != nil {
		return nil, fmt.Errorf("check existing records: %w", err)
	}
	slog.InfoContext(ctx, "looking for existing", slog.Any("toDelete", toDelete))
	deleteEps := make([]*endpoint.Endpoint, 0, len(toDelete))
	for _, record := range found {
		logger.DebugContext(ctx, "Checking endpoint", slog.Any("endpoint", record.DNSName))
		if _, ok := toDelete[record.DNSName]; ok && record.SetIdentifier != "" {
			logger.InfoContext(ctx, "found existing endpoint to delete",
				slog.Any("endpoint", record.DNSName), slog.Any("SetIdentifier", record.SetIdentifier))
			deleteEps = append(deleteEps, record)
		}
	}
	changes := &plan.Changes{
		Delete: deleteEps,
	}
	return changes, nil
}

func setDeleteCmdFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray(hostsFlag, []string{}, "FQDN for DNS entries to delete")
}
