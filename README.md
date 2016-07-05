# impl

impl is a tool to find implementers of an interface in Go programs.

```
$ ./impl -help
Find implementers of an interface in go source code.

Examples:
  impl -interface discovery.SwaggerSchemaInterface -path ./k8s.io/kubernetes/pkg/client/typed/discovery
  impl -interface datastore.RawInterface -path ~/go/src/github.com/luci/gae/service/datastore -format json 

Flags:
  -concrete-only
    	output concrete types only, by default the output contains both interface and concrete types that implement the specified interface
  -format string
    	output format, should be one of: {plain,json,xml} (default "plain")
  -interface string
    	interface name to find implementing types for, format: packageName.interfaceName
  -path string
    	absolute or relative path to directory to file
```

The `-interface` and `-path` flags are required.

The implementer type and interface type should both reside in the supplied path.

## Install

With Go installed:

```
go get -u github.com/nishanths/impl/...
```

## License

[MIT](https://nishanths.mit-license.org).