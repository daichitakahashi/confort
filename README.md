# Unit test with Docker containers in Go

[![Go Reference](https://pkg.go.dev/badge/github.com/daichitakahashi/confort.svg)](https://pkg.go.dev/github.com/daichitakahashi/confort)
[![coverage](https://img.shields.io/endpoint?style=flat-square&url=https%3A%2F%2Fdaichitakahashi.github.io%2Fconfort%2Fcoverage.json)](https://daichitakahashi.github.io/confort/coverage.html)

This project aims to use Docker containers in parallelized tests efficiently.  
Package `confort` provides test utilities to start Docker containers and generate unique identifiers.

## Features
### 1. Use Docker containers with `go test`
If you want to run a unit test which depends on Docker container, all you need
to do is declare the container in the test code with `confort.Confort` and run 
`go test`.  
The desired container will be started in the test. Also, the test code can reuse 
existing containers.

### 2. Share containers on parallelized tests
In some cases, starting a Docker container requires a certain amount of computing 
resources and time.  
Sometimes there are several unit tests that requires the same Docker container.
`confort.Confort` and `confort` command enables us to share it and make testing 
more efficient. And we can choose shared locking not only exclusive.

### 3. Avoid conflict between tests by using unique identifier generator
To efficiently use containers simultaneously from parallelized tests, it is 
effective to make the resource name created on the container unique for each test 
(e.g., database name or realm name).
`confort.Unique` helps generating unique names.

## Unit Test Example
### Single package test
```go
func TestExample(t *testing.T) {
    ctx := context.Background()
    
    // CFT_NAMESPACE=your_ci_id
    cft := confort.New(t, ctx,
        confort.WithNamespace("fallback-namespace", false),
    )
    
    // start container
    db := cft.Run(t, ctx, &confort.ContainerParams{
        Name:  "db",
        Image: "postgres:14.4-alpine3.16",
        Env: map[string]string{
            "POSTGRES_USER":     dbUser,
            "POSTGRES_PASSWORD": dbPassword,
        },
        ExposedPorts: []string{"5432/tcp"},
        Waiter:       wait.Healthy(),
    },
        // pull image if not exists
        confort.WithPullOptions(&types.ImagePullOptions{}, os.Stderr),
        // enable health check
        confort.WithContainerConfig(func(config *container.Config) {
            config.Healthcheck = &container.HealthConfig{
                Test:     []string{"CMD-SHELL", "pg_isready"},
                Interval: 5 * time.Second,
                Timeout:  3 * time.Second,
            }
        }),
    )
    
    // use container exclusively. the container will be released after the test finished
    // UseShared is also available
    ports := db.UseExclusive(t, ctx)
    addr := ports.HostPort("5432/tcp")
    // connect PostgreSQL using `addr`
	
    unique := confort.UniqueString(12)
    schema := unique.Must(t)
    // create a schema using `schema` as its name
}
```

### Parallelized test with `confort` command
```go
func TestExample(t *testing.T) {
    ctx := context.Background()

    // Connect beacon server using an address from `.confort.lock` or CFT_BEACON_ADDR.
    // The function does not fail even if the beacon server is not enabled. But beacon.Enabled == false.
    beacon := confort.ConnectBeacon(t, ctx)

    // CFT_NAMESPACE=your_ci_id
    cft := confort.New(t, ctx,
        confort.WithNamespace("fallback-namespace", false),
        // Following line enables an integration with `confort` command.
        // Exclusion control is performed through the beacon server.
        confort.WithBeacon(beacon),
    )

    // ...

    unique := confort.UniqueString(12,
        // Following line enables an integration with `confort` command. 
        // This stores the values created across the entire test
        // and helps create unique one.
        confort.WithGlobalUniqueness(beacon, "schema"), 
    )
    // ...
}
```

## Run test
### Unit test of a package
```shell
$ go test .
```

### Unit tests of all packages recursively
```shell
$ confort test -- ./...
```
`confort test` command launches *beacon server* that helps exclusion control of 
containers in parallelized tests and run test. Command line arguments after "--" 
are passed to `go test`. The `go` command is appropriately selected according to 
"go.mod" using [gocmd](https://github.com/daichitakahashi/gocmd).  
After tests finished, all created resources will be removed (removal policy is
configurable with option "-policy").

### In your CI script
Short example of *.gitlab-ci.yml*:
```yaml
variables:
  CFT_NAMESPACE: $CI_JOB_ID # use job id to avoid conflict with other tests
test:
  before_script:
    - confort start & # launch beacon server
  script:
    - go test ./... # run test using beacon server
  after_script:
    - confort stop # cleanup created Docker resources and stop beacon server safely
```

Off course, you can also use `confort test` command.

### Further detail
Please see the [package doc](https://pkg.go.dev/github.com/daichitakahashi/confort) and the help of `confort` command:
```shell
$ confort help [test/start/stop]
```
