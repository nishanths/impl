# impl

impl is a tool to find implementers of an interface in Go programs.

For example:

```
$ impl -interface discovery.SwaggerSchemaInterface -path $GOPATH/k8s.io/kubernetes/pkg/client/typed/discovery 
discovery_client.go:82:6: *discovery.DiscoveryClient
discovery_client.go:40:6: discovery.DiscoveryInterface
```

More options: 

```
$ impl -help
Find implementers of an interface in go source code.

Examples:
  impl -interface discovery.SwaggerSchemaInterface -path ./k8s.io/kubernetes/pkg/client/typed/discovery
  impl -interface datastore.RawInterface -path ~go/src/github.com/luci/gae/service/datastore -format json 

Flags:
  -concrete-only
    	output concrete types only, by default the output contains both interface and concrete types that implement the specified interface
  -format string
    	output format, should be one of: {plain,json,xml} (default "plain")
  -interface string
    	interface name to find implementing types for, format: packageName.interfaceName
  -path string
    	absolute or relative path to directory or file
```

The `-interface` and `-path` flags are required.

The implementer type and interface type should both reside in the supplied path.

Also see the [go oracle](https://godoc.org/golang.org/x/tools/cmd/oracle) for a similar, more machine-friendly tool. Unlike the oracle, impl directly takes the interface name as input instead of filename/byte offsets.

## Install

With Go installed:

```
go get -u github.com/nishanths/impl
```

## License

[MIT](https://nishanths.mit-license.org).