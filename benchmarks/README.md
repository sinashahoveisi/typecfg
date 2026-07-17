# Comparative benchmarks

Isolated Go module comparing typecfg's **reflection** bind/validate path
(`New[T]`) to the **generated** path (`NewGenerated[T]` + binders from
`typecfg-gen`). No third-party deps — only the root module.

## What is compared

| Pair | Struct | Scenario |
| ---- | ------ | -------- |
| `BenchmarkBind_Reflection` / `_Generated` | root `ParityConfig` | Valid full Load |
| `…_Nested` | local `DeepConfig` (3 nest levels) | Valid nested Load |
| `…_ValidationFailure` | root `ParityConfig` | Multi-field ValidationError |
| `…_LargeSlice` | local `LargeSliceConfig` | 1000-element native `[]any` ports |

`ParityConfig` / `ParityConfigBinder` are reused from the root module so the
happy-path and error-path benches measure the same representative surface as
the parity tests. Deep nesting and large-slice shapes live here so those
benches stay independent of the root test fixtures.

## Local development

The repo root [`go.work`](../go.work) includes this module. `go.mod` also has a
`replace` to `../` for runs outside the workspace.

## Reproduce

```bash
cd benchmarks
go test -bench=. -benchmem -count=3 ./...
```

Use `-count=3` (or more) — a single run is not enough to judge stability.

## Results

Captured on **2026-07-17** with **Go 1.22+** on **darwin/arm64** (Apple M1 Pro).

| Benchmark | ns/op (3-run range) | allocs/op | B/op |
| --------- | ------------------- | --------- | ---- |
| Reflection | 14632–14669 | 92 | 7630 |
| Generated | 8027–8198 | 83 | 7350 |
| Reflection Nested | 3736–3772 | 26 | 928 |
| Generated Nested | 984.5–1002 | 14 | 736 |
| Reflection ValidationFailure | 16201–16267 | 122 | 6134 |
| Generated ValidationFailure | 9205–9662 | 113 | 5910 |
| Reflection LargeSlice | 80809–81423 | 998 | ~11900 |
| Generated LargeSlice | 77566–77737 | 998 | ~28267 |

Raw output (verbatim `-count=3`):

```
goos: darwin
goarch: arm64
pkg: github.com/sinashahoveisi/typecfg/benchmarks
BenchmarkBind_Reflection-10                      	   78852	     14652 ns/op	    7630 B/op	      92 allocs/op
BenchmarkBind_Reflection-10                      	   81832	     14632 ns/op	    7630 B/op	      92 allocs/op
BenchmarkBind_Reflection-10                      	   82234	     14669 ns/op	    7630 B/op	      92 allocs/op
BenchmarkBind_Generated-10                       	  149068	      8027 ns/op	    7350 B/op	      83 allocs/op
BenchmarkBind_Generated-10                       	  145285	      8156 ns/op	    7350 B/op	      83 allocs/op
BenchmarkBind_Generated-10                       	  144986	      8198 ns/op	    7350 B/op	      83 allocs/op
BenchmarkBind_Reflection_Nested-10               	  315615	      3759 ns/op	     928 B/op	      26 allocs/op
BenchmarkBind_Reflection_Nested-10               	  317602	      3772 ns/op	     928 B/op	      26 allocs/op
BenchmarkBind_Reflection_Nested-10               	  317424	      3736 ns/op	     928 B/op	      26 allocs/op
BenchmarkBind_Generated_Nested-10                	 1203547	      1002 ns/op	     736 B/op	      14 allocs/op
BenchmarkBind_Generated_Nested-10                	 1205694	       984.5 ns/op	     736 B/op	      14 allocs/op
BenchmarkBind_Generated_Nested-10                	 1207410	       996.8 ns/op	     736 B/op	      14 allocs/op
BenchmarkBind_Reflection_ValidationFailure-10    	   73638	     16201 ns/op	    6134 B/op	     122 allocs/op
BenchmarkBind_Reflection_ValidationFailure-10    	   73270	     16223 ns/op	    6134 B/op	     122 allocs/op
BenchmarkBind_Reflection_ValidationFailure-10    	   74629	     16267 ns/op	    6134 B/op	     122 allocs/op
BenchmarkBind_Generated_ValidationFailure-10     	  129177	      9205 ns/op	    5910 B/op	     113 allocs/op
BenchmarkBind_Generated_ValidationFailure-10     	  130275	      9227 ns/op	    5910 B/op	     113 allocs/op
BenchmarkBind_Generated_ValidationFailure-10     	  130946	      9662 ns/op	    5910 B/op	     113 allocs/op
BenchmarkBind_Reflection_LargeSlice-10           	   14740	     81423 ns/op	   11901 B/op	     998 allocs/op
BenchmarkBind_Reflection_LargeSlice-10           	   14451	     81114 ns/op	   11901 B/op	     998 allocs/op
BenchmarkBind_Reflection_LargeSlice-10           	   14841	     80809 ns/op	   11900 B/op	     998 allocs/op
BenchmarkBind_Generated_LargeSlice-10            	   15502	     77692 ns/op	   28267 B/op	     998 allocs/op
BenchmarkBind_Generated_LargeSlice-10            	   15493	     77737 ns/op	   28267 B/op	     998 allocs/op
BenchmarkBind_Generated_LargeSlice-10            	   15456	     77566 ns/op	   28268 B/op	     998 allocs/op
PASS
ok  	github.com/sinashahoveisi/typecfg/benchmarks	41.714s
```

## Interpretation

Generated is consistently faster on the **ParityConfig** happy path (~1.8×)
and validation-failure path (~1.7×), with a small allocs/op drop (92→83,
122→113). The **nested** case shows the largest relative win (~3.8× time,
26→14 allocs) — reflection pays more for walking struct metadata.

**Large slices** are nearly tied on time (~4–5% faster generated) and
identical on allocs/op (998). Generated uses **~2.3× more bytes/op**
(~28KB vs ~12KB) because the emitted binder calls `CoerceIntSlice`, which
runs every element through an intermediate `[]string` (`rawToStringElems`)
before `ParseInt` into `[]int` — reflection’s `setSliceValue` formats and
parses each element straight into the destination slice and never retains
that string slice (~16B × N string headers explains most of the gap).

Absolute times are still tens of microseconds for a full config Load.
Config loading runs at startup or on reload — not in a per-request hot
loop. **For most users the reflection path is the right default**; the
codegen win is real but not large enough to justify the `go:generate`
workflow unless profiling shows bind/validate as a measured bottleneck
(deep trees, very frequent reloads, or alloc-sensitive environments).

These microbenchmarks do **not** include file/HTTP/Consul I/O, Watch
goroutines, or `SetRawValidator` / logger overhead.
