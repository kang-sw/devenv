---
name: cargo-brief
---

## Rust API Exploration

`cargo-brief` is available in PATH. Use it to explore Rust crate APIs — do not use WebSearch or WebFetch to browse docs.rust-lang.org when this tool is available.

```
cargo brief summary self                     # module overview — start here
cargo brief api self::some_module            # full API surface of a module
cargo brief search self SomeType             # find a type, function, or trait
cargo brief code fn some_function            # read source definition
cargo brief -C summary tokio@1               # explore a crates.io package
cargo brief -C features serde@1              # inspect feature flags
cargo brief -C -F rt,net api tokio@1 net     # feature-gated API
cargo brief lsp references some_function     # cross-crate call sites
cargo brief lsp blast-radius SomeType        # what breaks if this changes
```

Run `cargo-brief --help` for the full guide including `ts`, `examples`, and `lsp call-hierarchy`.
