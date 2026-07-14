# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Generic `Loader[T]` with `Load`, `Get`, `Watch`, and `Stop`
- Sources: `EnvSource` (core), `YAMLFile` and `JSONFile` (`sources/` module)
- Ordered source merging with later sources overriding earlier ones
- Reflection-based binding with `cfg` and `default` tags
- Validation: `required`, `min`, `max`, `oneof` plus `Validator` interface
- File hot reload via fsnotify with fallback to previous config on error
- Precise errors: `FieldError`, `ValidationError`, `SourceError`
- Presence tracking for `required` (distinguishes unset from explicit zero)
- Directory-based file watching for atomic rename/replace updates

### Changed

- File sources moved to `github.com/sinashahoveisi/typecfg/sources` submodule;
  core module has zero third-party dependencies
