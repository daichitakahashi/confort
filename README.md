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
`unique.Unique` helps generating unique names.

## Unit Test Example
### Single package test
```go
func TestExample(t *testing.T) {
    ctx := context.Background()
    
    // CFT_NAMESPACE=your_ci_id
    cft, err := confort.New(ctx,
        confort.WithNamespace("fallback-namespace", false),
    )
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() {
        _ = cft.Close()
    })

    // start container
    db, err := cft.Run(t, ctx, &confort.ContainerParams{
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
    if err != nil {
        t.Fatal(err)
    }
    
    // use container exclusively. the container will be released after the test finished
    // UseShared is also available
    ports, release, err := db.UseExclusive(ctx)
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(release)
    addr := ports.HostPort("5432/tcp")
    // connect PostgreSQL using `addr`
	
    uniq := unique.Must(unique.String(ctx, 12))
    schema := uniq.Must(t)
    // create a schema using `schema` as its name
}
```

### Parallelized test with `confort` command
```go
func TestExample(t *testing.T) {
    ctx := context.Background()

    // Connect beacon server using an address from `.confort.lock` or CFT_BEACON_ADDR.
    // The function does not fail even if the beacon server is not enabled. But beacon.Enabled == false.

    // CFT_NAMESPACE=your_ci_id
    cft, err := confort.New(ctx,
        confort.WithNamespace("fallback-namespace", false),
        // Following line enables an integration with `confort` command if it's available.
        // Connect will be established using an address of the beacon server from `.confort.lock` or CFT_BEACON_ADDR.
        // Exclusion control is performed through the beacon server.
        confort.WithBeacon(beacon),
    )

    // ...

    uniq := unique.Must(unique.String(ctx, 12,
        // Following line enables an integration with `confort` command too. 
        // This stores the values created across the entire test
        // and helps create unique one.
        unique.WithBeacon(t, ctx, "database-name"), 
    ))
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
  CONFORT: github.com/daichitakahashi/confort/cmd/confort
  CFT_NAMESPACE: $CI_JOB_ID # use job id to avoid conflict with other tests
test:
  script:
    - go run $CONFORT start & # launch beacon server
    - go test ./... # run test using beacon server
  after_script:
    - go run $CONFORT stop # cleanup created Docker resources and stop beacon server safely
```

Off course, you can also use `confort test` command.

## Detail of `confort` command
The functionality of this package consists of Go package `confort` and command `confort`.
These are communicating with each other in gRPC protocol, and each version should be matched.

To avoid version mismatches, "go run" is recommended instead of "go install".

### confort test
Start the beacon server and execute tests.  
After the tests are finished, the beacon server will be stopped automatically.  
If you want to use options of "go test", put them after "--".

There are following options.

#### `-go=<go version>`
Specify go version that runs tests.
"-go=mod" enables to use go version written in your `go.mod`.

#### `-go-mode=<mode>`
Specify detection mode of -go option. Default value is "fallback".
* "exact" finds go command that has the exact same version as given in "-go"
* "latest" finds the latest go command that has the same major version as given in "-go"
* "fallback" behaves like "latest", but if no command was found, fallbacks to "go" command

#### `-namespace=<namespace>`
Specify the namespace(prefix) of docker resources created by `confort.Confort`.
The value is set as `CFT_NAMESPACE`.

#### `-policy=<policy>`
Specify resource handling policy. The value is set as `CFT_RESOURCE_POLICY`. Default value is "reuse".
* With "error", the existing same resource(network and container) makes test failed
* With "reuse", tests reuse resources if already exist
* "reusable" is similar to "reuse", but created resources will not be removed after the tests finished
* "takeover" is also similar to "reuse", but reused resources will be removed after the tests

### confort start
Start the beacon server and output its endpoint to the lock file(".confort.lock"). If the lock file already exists, this command fails.  
See the document of `confort.WithBeacon`.

There is a following option.

#### `-lock-file=<filename>`
Specify the user-defined filename of the lock file. Default value is ".confort.lock".  
With this option, to tell the endpoint to the test code, you have to set file name as environment variable `CFT_LOCKFILE`.
If `CFT_LOCKFILE` is already set, the command uses the value as default.

### confort stop
Stop the beacon server started by `confort start` command.  
The target server address will be read from lock file(".confort.lock"), and the lock file will be removed.
If "confort start" has accompanied by "-lock-file" option, this command requires the same.

There is a following option.

#### `-lock-file=<filename>`
Specify the user-defined filename of the lock file. It is the same as the `-lock-file` option of `confort start`.
