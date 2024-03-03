# Boundation

Provides a CLI to manage OPNSense Unbound DNS overrides and a webservice implementing.
Originally intended as an [externalDNS](https://github.com/kubernetes-sigs/external-dns) webhook for managing DNS entries in OPNSense Unbound DNS. Webhook currently results in duplicate DNS entries being created.

## Limitations

Unbound does appear to support creating txt records. TXT records for external-dns ownership are stored in the Description field.

The description field has a hard limit of 255 chars.

## CLI

### CLI Install

```bash
go install github.com/MrUsefull/boundation/cmd/unbound@latest
```

### CLI Usage

Create or update overrides

```bash
unbound upsert --host=example.domain.here --target=1.2.3.4 --host=other.host.com --target=5.6.7.8
```

Read existing overrides

```bash
unbound read
```

Delete overrides

```bash
unbound delete --host=example.domain.here
```

Run interactive configuration menu

```bash
unbound configure
```

## Webservice

The webservice is indended to be used with the [externalDNS](https://github.com/kubernetes-sigs/external-dns) webhook system.

The current version of this project and external dns results in duplicate DNS entries constantly being created. It's recommended to use the CLI.
