package unbound

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

type cache struct {
	heritages map[string]string

	logger *slog.Logger
}

func newCache(logger *slog.Logger) *cache {
	return &cache{
		logger:    logger,
		heritages: make(map[string]string),
	}
}

func (c *cache) updateFromPlan(changes *plan.Changes) {
	c.removeRecords(changes.Delete)

	fromCreate := c.cacheFromSlice(append(changes.Create, changes.UpdateNew...))
	slog.Debug("updating cache", slog.Any("fromCreate", fromCreate))
	for k, v := range fromCreate {
		c.heritages[k] = v
	}

	slog.Debug("current cache", slog.Any("cache", c.heritages), slog.Any("plan", changes))
}

func (c *cache) updateReadRecords(read []*endpoint.Endpoint) {
	c.heritages = c.cacheFromSlice(read)
}

func (c *cache) cacheFromSlice(in []*endpoint.Endpoint) map[string]string {
	out := make(map[string]string)
	for _, create := range in {
		if create.RecordType == endpoint.RecordTypeTXT {
			out[create.DNSName] = create.Targets.String()
			if strings.HasPrefix(create.DNSName, "a-") {
				// this could be a case of a-service.foo.com being a real dns entry
				// or it could be externaldns prepending a- to the record.
				// Hard to tell which, so give our cache both
				name := strings.Replace(create.DNSName, "a-", "", 1)
				out[name] = create.Targets.String()
			}
		}
	}
	return out
}

func (c *cache) removeRecords(toDel []*endpoint.Endpoint) {
	for _, del := range toDel {
		if supportedType(del.RecordType) {
			delete(c.heritages, del.DNSName)
		}
	}
}

func (c *cache) createDescription(dnsName string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(c.heritages[dnsName]))
	return appendToDescription(encoded)
}

func appendToDescription(toAppend string) string {
	return fmt.Sprintf("%v %v", DescriptionPrefix, toAppend)
}
