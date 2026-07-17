# Platform Composition Boundary

`internal/platform` owns product-platform workflow composition. It does not own reusable capability implementation.

```text
admin.Graph              grouped active Admin HTTP services
admin.Build              one Admin capability assembly path for API runtime
retired.Graph            temporary App/Canvas transport dependencies; removed by P09
internal/module/*        reusable capability rules and persistence
```

Rules:

- `admin.Graph` groups services by responsibility; it does not introduce `ServiceImpl`, Manager, or catch-all interfaces.
- `admin.Build` receives validated runtime inputs and assembles services once. Routes consume the graph and never rebuild capabilities.
- `retired.Graph` is not an Admin contract source and must not be read by contract generation.
- Capability services remain independent of Gin and do not branch on product platform policy.
