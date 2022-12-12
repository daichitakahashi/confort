# Changelog

## [v1.2.1](https://github.com/daichitakahashi/confort/compare/v1.2.0...v1.2.1) - 2022-12-12
- Bump github.com/daichitakahashi/testingc from 0.0.1 to 0.0.2 by @dependabot in https://github.com/daichitakahashi/confort/pull/63
- Bump google.golang.org/grpc from 1.50.1 to 1.51.0 by @dependabot in https://github.com/daichitakahashi/confort/pull/65
- Bump github.com/daichitakahashi/gocmd from 1.0.2 to 1.0.4 by @dependabot in https://github.com/daichitakahashi/confort/pull/66

## [v1.2.0](https://github.com/daichitakahashi/confort/compare/v1.1.0...v1.2.0) - 2022-11-05
- Update confort: do not call createContainer in `Container.Use` by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/54
- Bump github.com/daichitakahashi/gocmd: v1.0.1 -> v1.0.2 by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/56
- Dependabot enabled by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/57
- Bump github.com/jackc/pgconn from 1.12.1 to 1.13.0 by @dependabot in https://github.com/daichitakahashi/confort/pull/62
- Bump github.com/jackc/pgx/v4 from 4.16.1 to 4.17.2 by @dependabot in https://github.com/daichitakahashi/confort/pull/59
- Bump google.golang.org/grpc from 1.49.0 to 1.50.1 by @dependabot in https://github.com/daichitakahashi/confort/pull/61
- Bump github.com/docker/docker from 20.10.18+incompatible to 20.10.21+incompatible by @dependabot in https://github.com/daichitakahashi/confort/pull/60
- Bump github.com/docker/cli from 20.10.18+incompatible to 20.10.21+incompatible by @dependabot in https://github.com/daichitakahashi/confort/pull/58
- Add ability to acquire locks of multi-container to avoid deadlocks by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/55

## [v1.1.0](https://github.com/daichitakahashi/confort/compare/v1.0.0...v1.1.0) - 2022-10-11
- Remove `testing.TB` from arguments and return error by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/47
- Update unique: add `unique.Must` by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/48
- Bump github.com/daichitakahashi/gocmd: v1.0.0 -> v1.0.1 by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/49
- Fix flaky by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/50

## [v1.0.0](https://github.com/daichitakahashi/confort/compare/v0.4.1...v1.0.0) - 2022-10-02
- Update internal/cmd: `confort test` should handle SIGTERM and SIGINT to release created resources by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/34
- Unexport beacon by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/40
- `beacon.Connection` should be singleton by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/41
- Update internal/beacon: add an option to disable the integration with beacon server explicitly by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/42
- Add internal/logging: optional logging by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/44
- Disable config consistency by default by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/45

## [v0.4.1](https://github.com/daichitakahashi/confort/compare/v0.4.0...v0.4.1) - 2022-09-24
- More documents on README by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/33

## [v0.4.0](https://github.com/daichitakahashi/confort/compare/v0.3.0...v0.4.0) - 2022-09-22
- Add package `unique` and `wait` by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/30

## [v0.3.0](https://github.com/daichitakahashi/confort/compare/v0.2.0...v0.3.0) - 2022-09-21
- [skip ci] Update README.md by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/27
- Bump some dependencies by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/28
- Declarative container by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/29

## [v0.2.0](https://github.com/daichitakahashi/confort/compare/v0.1.0...v0.2.0) - 2022-08-30
- Init once by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/26

## [v0.1.0](https://github.com/daichitakahashi/confort/commits/v0.1.0) - 2022-08-24
- Rewrite by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/10
- Implement beacon server by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/11
- Integrate by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/17
- Use Github Actions (test, deploy-coverage) by @daichitakahashi in https://github.com/daichitakahashi/confort/pull/19
