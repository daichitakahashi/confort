package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/daichitakahashi/confort"
	"github.com/daichitakahashi/confort/internal/beaconutil"
	"github.com/daichitakahashi/confort/internal/mock"
	"github.com/daichitakahashi/confort/proto/beacon"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/go-connections/nat"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
)

func TestBeaconServer_Register(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, cli, _, _ := startServer(t)

	namespace := unique.Must(t)

	// register namespace and client
	registerResp, err := cli.Register(ctx, &beacon.RegisterRequest{
		Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registerResp.ClientId == "" {
		t.Fatal("empty clientID on Register response")
	}
	var nw types.NetworkResource
	err = json.Unmarshal(registerResp.NetworkResource, &nw)
	if err != nil {
		t.Fatal(err)
	}
	if nw.Name != namespace {
		t.Fatalf("expected namespace %q, but got %q", namespace, nw.Name)
	}

	// register another client (reuse namespace)
	anotherResp, err := cli.Register(ctx, &beacon.RegisterRequest{
		Namespace: namespace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if registerResp.ClientId == anotherResp.ClientId {
		t.Fatal("expected to be registered another client, but got same clientID")
	}

	// check reuse network
	var nw2 types.NetworkResource
	err = json.Unmarshal(anotherResp.NetworkResource, &nw2)
	if err != nil {
		t.Fatal(err)
	}
	if nw2.Name != namespace {
		t.Fatalf("expected namespace %q, but got %q", namespace, nw.Name)
	}
	if nw.ID != nw2.ID {
		t.Fatal("expected to reuse same network, but got different network ID")
	}

	// deregister
	_, err = cli.Deregister(ctx, &beacon.DeregisterRequest{
		ClientId: registerResp.ClientId,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = cli.Deregister(ctx, &beacon.DeregisterRequest{
		ClientId: anotherResp.ClientId,
	})
	if err != nil {
		t.Fatal(err)
	}

	// error on re-deregister
	_, err = cli.Deregister(ctx, &beacon.DeregisterRequest{
		ClientId: registerResp.ClientId,
	})
	if err == nil {
		t.Fatal("error expected but succeeded")
	}
}

func stringPtr(v string) *string {
	return &v
}

func TestBeaconServer_BuildImage(t *testing.T) {
	t.Parallel()

	// prepare input
	buildContext := "ある人びとは、「オドラデク」という言葉はスラヴ語から出ている、といって、それを根拠にしてこの言葉の成立を証明しようとしている。ほかの人びとはまた、この言葉はドイツ語から出ているものであり、ただスラヴ語の影響を受けているだけだ、といっている。この二つの解釈が不確かなことは、どちらもあたってはいないという結論を下してもきっと正しいのだ、と思わせる。ことに、そのどちらの解釈によっても言葉の意味が見出せられないのだから、なおさらのことだ。"
	buildOptions := types.ImageBuildOptions{
		Tags:        []string{"registry/container:tag"},
		Remove:      true,
		ForceRemove: true,
		Dockerfile:  "../Dockerfile",
		BuildArgs: map[string]*string{
			"foo":  stringPtr("bar"),
			"john": stringPtr("doe"),
			"hoge": nil,
		},
	}
	const force = true

	buildRequest := &beacon.BuildImageRequest{
		Build: &beacon.BuildImageRequest_BuildInfo{
			BuildInfo: &beacon.BuildInfo{
				BuildOptions: beaconutil.ConvertBuildOptionsToProto(buildOptions),
				Force:        force,
			},
		},
	}

	// build function that inject to backend
	//  - check if input has sent correctly
	//  - copy buildContext to buildOut
	buildImageFunc := func(ctx context.Context, buildContext io.Reader, _buildOptions types.ImageBuildOptions, _force bool, buildOut io.Writer) error {
		diff := cmp.Diff(buildOptions, _buildOptions, cmpopts.EquateEmpty())
		if diff != "" {
			return fmt.Errorf("unexpected ImageBuildOptions\n%s", diff)
		}
		if force != _force {
			return fmt.Errorf("expected force option=%t, but got %t", force, _force)
		}
		_, err := io.Copy(buildOut, buildContext)
		return err
	}

	testCases := []struct {
		desc                   string
		buildImageFunc         func(ctx context.Context, buildContext io.Reader, _buildOptions types.ImageBuildOptions, _force bool, buildOut io.Writer) error
		firstBuildImageRequest *beacon.BuildImageRequest
		checkResult            func(t *testing.T, err error, buildOut *strings.Builder)
	}{
		{
			desc: "success",

			buildImageFunc:         buildImageFunc,
			firstBuildImageRequest: buildRequest,
			checkResult: func(t *testing.T, err error, buildOut *strings.Builder) {
				if err != nil {
					t.Fatal(err)
				}
				// check build output
				if buildOut.String() != buildContext {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "invalid procedure call",

			buildImageFunc: buildImageFunc,
			firstBuildImageRequest: &beacon.BuildImageRequest{
				Build: nil,
			},
			checkResult: func(t *testing.T, err error, _ *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
		},
		{
			desc: "error during build on backend",

			buildImageFunc: func(ctx context.Context, buildContext io.Reader, buildOptions types.ImageBuildOptions, force bool, buildOut io.Writer) error {
				discard := make([]byte, 100)
				_, err := io.ReadFull(buildContext, discard)
				if err != nil {
					return err
				}
				_, err = buildOut.Write(discard)
				if err != nil {
					return err
				}
				// return error after read 100 bytes
				return errors.New("unknown error")
			},
			firstBuildImageRequest: buildRequest,
			checkResult: func(t *testing.T, err error, buildOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}

				// check build output
				if buildOut.String() != buildContext[:100] {
					t.Fatal("got unexpected build output")
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			be, cli, _, _ := startServer(t)
			be.BuildImageFunc = tc.buildImageFunc

			// start build stream
			stream, err := cli.BuildImage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			// first, send input
			err = stream.Send(tc.firstBuildImageRequest)
			if err != nil {
				_ = stream.CloseSend()
				t.Fatal(err)
			}

			buildContextReader := strings.NewReader(buildContext)
			buildOut := &strings.Builder{}
			eg, ctx := errgroup.WithContext(ctx)

			// receive output stream
			eg.Go(func() error {
				return beaconutil.ReceiveStream[beacon.BuildImageResponse](
					ctx, stream, func(resp *beacon.BuildImageResponse) (completed bool, _ error) {
						switch vt := resp.GetProcessing().(type) {
						case *beacon.BuildImageResponse_Building:
							_, err = buildOut.Write(vt.Building.GetMessage())
							return false, err
						case *beacon.BuildImageResponse_Built:
							return true, nil
						}
						panic("unreachable")
					},
				)
			})
			// send build context stream
			eg.Go(func() error {
				buf := make([]byte, 4*1024)
				for {
					select {
					case <-ctx.Done():
						return multierr.Append(ctx.Err(), stream.CloseSend())
					default:
					}
					n, err := buildContextReader.Read(buf)
					if n > 0 {
						err = stream.Send(&beacon.BuildImageRequest{
							Build: &beacon.BuildImageRequest_Context{
								Context: buf[:n],
							},
						})
						if err != nil {
							return multierr.Append(err, stream.CloseSend())
						}
					}
					if err == io.EOF {
						return stream.CloseSend()
					}
					if err != nil {
						return multierr.Append(err, stream.CloseSend())
					}
				}
			})
			err = eg.Wait()
			tc.checkResult(t, err, buildOut)
		})
	}
}

func marshal(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func TestBeaconServer_CreateContainer(t *testing.T) {
	t.Parallel()

	const image = "registry/repository:tag"
	pullOutput := "ある朝、グレゴール・ザムザが気がかりな夢から目ざめたとき、自分がベッドの上で一匹の巨大な毒虫に変ってしまっているのに気づいた。彼は甲殻のように固い背中を下にして横たわり、頭を少し上げると、何本もの弓形のすじにわかれてこんもりと盛り上がっている自分の茶色の腹が見えた。腹の盛り上がりの上には、かけぶとんがすっかりずり落ちそうになって、まだやっともちこたえていた。ふだんの大きさに比べると情けないくらいかぼそいたくさんの足が自分の眼の前にしょんぼりと光っていた。"

	cc := container.Config{
		Image: image,
		ExposedPorts: nat.PortSet{
			"8080/tcp": struct{}{},
		},
		Env: []string{"A=a", "B=b"},
		Healthcheck: &container.HealthConfig{
			Test:        []string{"curl", "-f", "https://example.com"},
			Interval:    time.Minute,
			Timeout:     time.Second * 30,
			StartPeriod: time.Second * 30,
			Retries:     5,
		},
	}
	hc := container.HostConfig{
		PortBindings: nat.PortMap{
			"8080/tcp": nil,
		},
		RestartPolicy: container.RestartPolicy{
			Name:              "always",
			MaximumRetryCount: 3,
		},
		AutoRemove: true,
	}
	nc := network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{},
	}
	pc := types.ImagePullOptions{
		All: true,
	}

	defaultCreateContainerFunc := func(ctx context.Context, name string,
		_cc *container.Config, _hc *container.HostConfig, _nc *network.NetworkingConfig, configConsistency bool,
		pullOptions *types.ImagePullOptions, pullOut io.Writer,
	) (string, error) {
		if diff := cmp.Diff(&cc, _cc, cmpopts.EquateEmpty()); diff != "" {
			return "", fmt.Errorf("unexpected container config\n%s", diff)
		}
		if diff := cmp.Diff(&hc, _hc, cmpopts.EquateEmpty()); diff != "" {
			return "", fmt.Errorf("unexpected host config\n%s", diff)
		}
		if diff := cmp.Diff(&nc, _nc, cmpopts.EquateEmpty()); diff != "" {
			return "", fmt.Errorf("unexpected network config\n%s", diff)
		}
		var err error
		if pullOptions != nil {
			if diff := cmp.Diff(&pc, pullOptions, cmpopts.EquateEmpty()); diff != "" {
				return "", fmt.Errorf("unexpected pull options\n%s", diff)
			}
			_, err = pullOut.Write([]byte(pullOutput))
		}
		return "container_id", err

	}

	defaultCreateContainerRequest := func(clientID string) *beacon.CreateContainerRequest {
		return &beacon.CreateContainerRequest{
			ClientId:               clientID,
			Name:                   "test-container-name",
			ContainerConfig:        marshal(cc),
			HostConfig:             marshal(hc),
			NetworkingConfig:       marshal(nc),
			CheckConfigConsistency: true,
			PullOptions:            beaconutil.ConvertPullOptionsToProto(&pc),
		}
	}

	testCases := []struct {
		desc                   string
		createContainerFunc    func(ctx context.Context, name string, cc *container.Config, hc *container.HostConfig, nc *network.NetworkingConfig, configConsistency bool, pullOptions *types.ImagePullOptions, pullOut io.Writer) (string, error)
		createContainerRequest func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest
		checkResult            func(t *testing.T, err error, pullOut *strings.Builder)
	}{
		{
			desc: "success with pull",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err != nil {
					t.Fatal(err)
				}
				if pullOut.String() != pullOutput {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "success without pull",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				req.PullOptions = nil
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err != nil {
					t.Fatal(err)
				}
				if pullOut.String() != "" {
					t.Fatalf("got unexpected build output\n%s", pullOut.String())
				}
			},
		},
		{
			desc: "unknown clientID",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				req.ClientId += "_invalid"
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
				if pullOut.String() != "" {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "invalid container config",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				req.ContainerConfig = marshal(999)
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
				if pullOut.String() != "" {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "invalid host config",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				req.HostConfig = marshal("hhoosstt")
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
				if pullOut.String() != "" {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "invalid network config",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				req.NetworkingConfig = marshal([]int{1, 2, 3})
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
				if pullOut.String() != "" {
					t.Fatal("got unexpected build output")
				}
			},
		},
		{
			desc: "error on backend",

			createContainerFunc: defaultCreateContainerFunc,
			createContainerRequest: func(req *beacon.CreateContainerRequest) *beacon.CreateContainerRequest {
				cc := cc
				cc.Image = "image:unknown"
				req.ContainerConfig = marshal(cc)
				return req
			},
			checkResult: func(t *testing.T, err error, pullOut *strings.Builder) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
				if pullOut.String() != "" {
					t.Fatal("got unexpected build output")
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			be, cli, _, _ := startServer(t)
			be.NamespaceFunc = func(ctx context.Context, namespace string) (confort.Namespace, error) {
				ns, _ := defaultNamespaceFunc(ctx, namespace)
				ns.(*mock.NamespaceMock).CreateContainerFunc = tc.createContainerFunc
				return ns, nil
			}

			resp, err := cli.Register(ctx, &beacon.RegisterRequest{
				Namespace: unique.Must(t),
			})
			if err != nil {
				t.Fatal(err)
			}

			req := defaultCreateContainerRequest(resp.ClientId)
			stream, err := cli.CreateContainer(ctx, tc.createContainerRequest(req))
			if err != nil {
				t.Fatal(err)
			}

			pullOut := &strings.Builder{}
			err = beaconutil.ReceiveStream[beacon.CreateContainerResponse](
				ctx, stream, func(resp *beacon.CreateContainerResponse) (completed bool, _ error) {
					switch vt := resp.Processing.(type) {
					case *beacon.CreateContainerResponse_Pulling:
						_, err := pullOut.Write(vt.Pulling.GetMessage())
						return false, err
					case *beacon.CreateContainerResponse_Created:
						return true, nil
					}
					panic("unreachable")
				},
			)
			tc.checkResult(t, err, pullOut)
		})
	}
}

func TestBeaconServer_AcquireContainerEndpoint(t *testing.T) {
	t.Parallel()

	const containerName = "container-name"
	const defaultExclusive = true
	defaultAcquireRequest := func(clientID string) *beacon.AcquireContainerEndpointRequest {
		return &beacon.AcquireContainerEndpointRequest{
			ClientId:      clientID,
			ContainerName: containerName,
			Exclusive:     defaultExclusive,
		}
	}

	const (
		defaultHostIP   = "127.0.0.1"
		defaultHostPort = "8080"
	)
	defaultStartContainerFunc := func(ctx context.Context, name string, exclusive bool) (nat.PortMap, error) {
		if name != containerName {
			return nil, errors.New("unknown container name")
		}
		return nat.PortMap{
			"80/tcp": []nat.PortBinding{
				{
					HostIP:   defaultHostIP,
					HostPort: defaultHostPort,
				},
			},
		}, nil
	}
	defaultCheckCreateContainerResult := func(t *testing.T, endpoints map[string]*beacon.Endpoints, err error) {
		if err != nil {
			t.Fatal(err)
		}
		bindings := endpoints["80/tcp"].GetBindings()
		if len(bindings) != 1 {
			t.Fatal("unexpected endpoints")
		}
		for _, binding := range bindings {
			if binding.HostIp != defaultHostIP {
				t.Fatalf("unexpected host ip: %q", binding.HostIp)
			}
			if binding.HostPort != defaultHostPort {
				t.Fatalf("unexpected host port: %q", binding.HostPort)
			}
		}
	}

	defaultReleaseContainerRequest := func(clientID string) *beacon.ReleaseContainerRequest {
		return &beacon.ReleaseContainerRequest{
			ClientId:      clientID,
			ContainerName: containerName,
		}
	}

	defaultReleaseContainerFunc := func(ctx context.Context, name string, exclusive bool) error {
		if name != containerName {
			return errors.New("unknown container name")
		}
		return nil
	}

	testCases := []struct {
		desc                        string
		acquireRequest              func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest
		startContainerFunc          func(ctx context.Context, name string, exclusive bool) (nat.PortMap, error)
		checkCreateContainerResult  func(t *testing.T, endpoints map[string]*beacon.Endpoints, err error)
		releaseRequest              func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest
		releaseContainerFunc        func(ctx context.Context, name string, exclusive bool) error
		checkReleaseContainerResult func(t *testing.T, err error)
	}{
		{
			desc: "success",

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				return req
			},
			startContainerFunc:         defaultStartContainerFunc,
			checkCreateContainerResult: defaultCheckCreateContainerResult,
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				return req
			},
			releaseContainerFunc: defaultReleaseContainerFunc,
			checkReleaseContainerResult: func(t *testing.T, err error) {
				if err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			desc: "unknown clientID",

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				req.ClientId += "_unknown"
				return req
			},
			startContainerFunc: defaultStartContainerFunc,
			checkCreateContainerResult: func(t *testing.T, endpoints map[string]*beacon.Endpoints, err error) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				return req
			},
			releaseContainerFunc: defaultReleaseContainerFunc,
			checkReleaseContainerResult: func(t *testing.T, err error) {
				t.Fatal("unreachable")
			},
		},
		{
			desc: "error on backend", // unknown container name

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				req.ContainerName += "_unknown"
				return req
			},
			startContainerFunc: defaultStartContainerFunc,
			checkCreateContainerResult: func(t *testing.T, endpoints map[string]*beacon.Endpoints, err error) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				return req
			},
			releaseContainerFunc: defaultReleaseContainerFunc,
			checkReleaseContainerResult: func(t *testing.T, err error) {
				t.Fatal("unreachable")
			},
		},
		{
			desc: "error on ReleaseContainer(unknown clientID)",

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				return req
			},
			startContainerFunc:         defaultStartContainerFunc,
			checkCreateContainerResult: defaultCheckCreateContainerResult,
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				req.ClientId += "_unknown"
				return req
			},
			releaseContainerFunc: defaultReleaseContainerFunc,
			checkReleaseContainerResult: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
		},
		{
			desc: "error on ReleaseContainer(unknown containerName)",

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				return req
			},
			startContainerFunc:         defaultStartContainerFunc,
			checkCreateContainerResult: defaultCheckCreateContainerResult,
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				req.ContainerName += "_unknown"
				return req
			},
			releaseContainerFunc: defaultReleaseContainerFunc,
			checkReleaseContainerResult: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
		},
		{
			desc: "error on ReleaseContainer(on backend)",

			acquireRequest: func(req *beacon.AcquireContainerEndpointRequest) *beacon.AcquireContainerEndpointRequest {
				return req
			},
			startContainerFunc:         defaultStartContainerFunc,
			checkCreateContainerResult: defaultCheckCreateContainerResult,
			releaseRequest: func(req *beacon.ReleaseContainerRequest) *beacon.ReleaseContainerRequest {
				return req
			},
			releaseContainerFunc: func(ctx context.Context, name string, exclusive bool) error {
				return errors.New("unknown error")
			},
			checkReleaseContainerResult: func(t *testing.T, err error) {
				if err == nil {
					t.Fatal("error expected but succeeded")
				}
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		if tc.checkCreateContainerResult == nil {
			continue
		}
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			be, cli, _, _ := startServer(t)
			be.NamespaceFunc = func(ctx context.Context, namespace string) (confort.Namespace, error) {
				ns, _ := defaultNamespaceFunc(ctx, namespace)
				mk := ns.(*mock.NamespaceMock)
				mk.StartContainerFunc = tc.startContainerFunc
				mk.ReleaseContainerFunc = tc.releaseContainerFunc
				return ns, nil
			}

			registerResp, err := cli.Register(ctx, &beacon.RegisterRequest{
				Namespace: unique.Must(t),
			})
			if err != nil {
				t.Fatal(err)
			}
			req := defaultAcquireRequest(registerResp.ClientId)
			stream, err := cli.AcquireContainerEndpoint(ctx, tc.acquireRequest(req))
			if err != nil {
				t.Fatal(err)
			}
			acquireResp, err := stream.Recv()

			tc.checkCreateContainerResult(t, acquireResp.GetEndpoints(), err)
			if err != nil {
				return
			}

			releaseReq := defaultReleaseContainerRequest(registerResp.ClientId)
			_, err = cli.ReleaseContainer(ctx, tc.releaseRequest(releaseReq))

			tc.checkReleaseContainerResult(t, err)
		})
	}
}

func TestBeaconServer_Shutdown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	be, cli, _, _ := startServer(t)
	be.NamespaceFunc = func(ctx context.Context, namespace string) (confort.Namespace, error) {
		ns, _ := defaultNamespaceFunc(ctx, namespace)
		ns.(*mock.NamespaceMock).ReleaseFunc = func(ctx context.Context) error {
			return fmt.Errorf("dummy error: %s", namespace)
		}
		return ns, nil
	}

	for i := 0; i < 5; i++ {
		_, err := cli.Register(ctx, &beacon.RegisterRequest{
			Namespace: unique.Must(t),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// beacon.Server.Shutdown will make error
}
