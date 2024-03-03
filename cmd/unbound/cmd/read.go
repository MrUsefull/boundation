package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/MrUsefull/boundation/internal/config"
	"github.com/MrUsefull/boundation/internal/unbound"
	"github.com/spf13/cobra"
	"sigs.k8s.io/external-dns/endpoint"
)

var readCMD = &cobra.Command{
	Use:     "read",
	Short:   "Shows existing overrides in OPNSense unbound DNS",
	Example: "read",
	RunE:    configured(runRead),
}

func runRead(cmd *cobra.Command, _ []string) error {
	return readEndpoints(http.DefaultClient, pkgConfig, os.Stdout, cmd)
}

func readEndpoints(client *http.Client, cfg config.Config, output io.Writer, cmd *cobra.Command) error {
	ctx := cmd.Context()
	provider := unbound.New(client, cfg, logger)
	found, err := provider.Records(ctx)
	if err != nil {
		return fmt.Errorf("read records: %w", err)
	}
	printEndpoints(output, found)
	return nil
}

func printEndpoints(w io.Writer, endpoints []*endpoint.Endpoint) {
	maxLen := getMaxDnsnameLen(endpoints)
	writer := tabwriter.NewWriter(w, maxLen, 5, 5, ' ', 0)
	fmt.Fprintf(writer, "\n")
	fmt.Fprint(writer, "DNS Name\tTarget\tRecord Type\t\n")
	for _, ep := range endpoints {
		fmt.Fprintf(writer, "%v\t%v\t%v\n", ep.DNSName, ep.Targets, ep.RecordType)
	}
	writer.Flush()
}

func getMaxDnsnameLen(endpoints []*endpoint.Endpoint) int {
	max := 0
	for _, ep := range endpoints {
		if len(ep.DNSName) > max {
			max = len(ep.Targets[0])
		}
	}
	return max
}
