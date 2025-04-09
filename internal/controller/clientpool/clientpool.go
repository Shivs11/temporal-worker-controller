// Unless explicitly stated otherwise all files in this repository are licensed under the MIT License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/). Copyright 2024 Datadog, Inc.

package clientpool

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"fmt"
	"os"
	"sync"

	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/log"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/temporal-worker-controller/api/v1alpha1"
)

type ClientPoolKey struct {
	HostPort  string
	Namespace string
}

type ClientPool struct {
	mux       sync.RWMutex
	logger    log.Logger
	clients   map[ClientPoolKey]sdkclient.Client
	k8sClient runtimeclient.Client
}

func New(l log.Logger, c runtimeclient.Client) *ClientPool {
	return &ClientPool{
		logger:    l,
		clients:   make(map[ClientPoolKey]sdkclient.Client),
		k8sClient: c,
	}
}

func (cp *ClientPool) GetSDKClient(key ClientPoolKey) (sdkclient.Client, bool) {
	cp.mux.RLock()
	defer cp.mux.RUnlock()

	c, ok := cp.clients[key]
	if ok {
		return c, true
	}
	return nil, false
}

func SystemCertPool() (*x509.CertPool, error) {
	if sysRoots := systemRootsPool(); sysRoots != nil {
		return sysRoots.Clone(), nil
	}

	return loadSystemRoots()
}

func systemRootsPool() *x509.CertPool {
	pool, err := x509.SystemCertPool()
	if err != nil {
		return nil
	}
	return pool
}

func loadSystemRoots() (*x509.CertPool, error) {
	return x509.SystemCertPool()
}

type NewClientOptions struct {
	TemporalNamespace string
	K8sNamespace      string
	Spec              v1alpha1.TemporalConnectionSpec
}

func (cp *ClientPool) UpsertClient(ctx context.Context, opts NewClientOptions) (sdkclient.Client, error) {
	fmt.Println("Temporal Namespace: ", opts.TemporalNamespace)
	fmt.Println("Host Port: ", opts.Spec.HostPort)
	fmt.Println("MutualTLSSecret: ", opts.Spec.MutualTLSSecret)

	clientOpts := sdkclient.Options{
		Logger:    cp.logger,
		HostPort:  opts.Spec.HostPort,
		Namespace: opts.TemporalNamespace,
		// TODO(jlegrone): fix this
		Credentials: sdkclient.NewAPIKeyStaticCredentials(os.Getenv("TEMPORAL_CLOUD_API_KEY")),
		//Credentials: client.NewAPIKeyDynamicCredentials(func(ctx context.Context) (string, error) {
		//	token, ok := os.LookupEnv("TEMPORAL_CLOUD_API_KEY")
		//	if ok {
		//		if token == "" {
		//			return "", fmt.Errorf("empty token")
		//		}
		//		return token, nil
		//	}
		//	return "", fmt.Errorf("token not found")
		//}),
		//Credentials: client.NewMTLSCredentials(tls.Certificate{
		//	Certificate:                  cert.Certificate,
		//	PrivateKey:                   cert.PrivateKey,
		//	SupportedSignatureAlgorithms: nil,
		//	OCSPStaple:                   nil,
		//	SignedCertificateTimestamps:  nil,
		//	Leaf:                         nil,
		//}),
	}
	// Get the connection secret if it exists
	if opts.Spec.MutualTLSSecret != "" {
		var secret corev1.Secret
		if err := cp.k8sClient.Get(ctx, types.NamespacedName{
			Name:      opts.Spec.MutualTLSSecret,
			Namespace: opts.K8sNamespace,
		}, &secret); err != nil {
			return nil, err
		}
		if secret.Type != corev1.SecretTypeTLS {
			err := fmt.Errorf("secret %s must be of type kubernetes.io/tls", secret.Name)
			return nil, err
		}

		fmt.Println("Length of tls.crt: ", len(secret.Data["tls.crt"]))
		fmt.Println("Length of tls.key: ", len(secret.Data["tls.key"]))
		fmt.Println("Length of ca.crt: ", len(secret.Data["ca.crt"]))

		cert, err := tls.X509KeyPair(secret.Data["tls.crt"], secret.Data["tls.key"])
		if err != nil {
			return nil, err
		}

		caCertPool, err := SystemCertPool()
		if err != nil {
			return nil, err
		}

		ok := caCertPool.AppendCertsFromPEM(secret.Data["ca.crt"])
		if !ok {
			return nil, fmt.Errorf("failed to parse ca.crt from secret %s", secret.Name)
		}

		clientOpts.ConnectionOptions.TLS = &tls.Config{
			RootCAs:            caCertPool,
			InsecureSkipVerify: true,
			GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
				return &cert, nil
			},
		}
	}

	c, err := sdkclient.Dial(clientOpts)
	if err != nil {
		return nil, err
	}

	if _, err := c.CheckHealth(context.Background(), &sdkclient.CheckHealthRequest{}); err != nil {
		panic(err)
	}

	cp.mux.Lock()
	defer cp.mux.Unlock()

	key := ClientPoolKey{
		HostPort:  opts.Spec.HostPort,
		Namespace: opts.TemporalNamespace,
	}
	cp.clients[key] = c

	return c, nil
}

func (cp *ClientPool) Close() {
	cp.mux.Lock()
	defer cp.mux.Unlock()

	for _, c := range cp.clients {
		c.Close()
	}

	cp.clients = make(map[ClientPoolKey]sdkclient.Client)
}
