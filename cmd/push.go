package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/go-logr/stdr"
	"github.com/prometheus/client_golang/prometheus/push"
	"github.com/raffis/renovate-metrics/pkg/parser"
	"github.com/spf13/cobra"
)

// Environment variables used to configure authentication against the prometheus
// push gateway. They are read at command execution time, never as flag defaults,
// so that secrets can not leak into the generated command help.
const (
	envPrometheusUsername    = "RENOVATE_METRICS_PROMETHEUS_USERNAME"
	envPrometheusPassword    = "RENOVATE_METRICS_PROMETHEUS_PASSWORD"
	envPrometheusBearerToken = "RENOVATE_METRICS_PROMETHEUS_BEARER_TOKEN"
)

func newStdLogger(flags int) stdr.StdLogger {
	return log.New(os.Stdout, "", flags)
}

// requestOrigin returns the scheme and host of u in a comparable form.
func requestOrigin(u *url.URL) string {
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// pushGatewayOrigin returns the origin the pusher will send its requests to. It
// applies the same scheme defaulting as push.New so the origin matches the
// requests the pusher actually creates. Neither the URL nor the parse error is
// part of the returned error because the URL may carry credentials in its
// userinfo.
func pushGatewayOrigin(gatewayURL string) (string, error) {
	if !strings.Contains(gatewayURL, "://") {
		gatewayURL = "http://" + gatewayURL
	}

	parsed, err := url.Parse(gatewayURL)
	if err != nil {
		return "", errors.New("prometheus push gateway URL is not a valid URL")
	}

	if parsed.Host == "" {
		return "", errors.New("prometheus push gateway URL has no host")
	}

	return requestOrigin(parsed), nil
}

// bearerAuthRoundTripper adds an Authorization header with a bearer token to
// every request addressed to the push gateway. The request is cloned before the
// header is set so neither the original request nor the wrapped transport is
// mutated.
//
// The header is only set for the configured origin. A round tripper runs once
// per redirect hop, after net/http has already stripped the credentials it
// manages itself, so an unconditional header would hand the token to whatever
// host the gateway redirects to (and to a plain http:// target on a scheme
// downgrade). Scoping it to the origin gives the bearer token the same
// protection net/http gives the basic auth credentials set by push.Pusher.
type bearerAuthRoundTripper struct {
	token  string
	origin string
	rt     http.RoundTripper
}

func (t *bearerAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.rt
	if rt == nil {
		rt = http.DefaultTransport
	}

	if requestOrigin(req.URL) != t.origin {
		return rt.RoundTrip(req)
	}

	req = req.Clone(req.Context())
	req.Header.Set("Authorization", "Bearer "+t.token)

	return rt.RoundTrip(req)
}

// prometheusAuth is the resolved authentication configuration for the prometheus
// push gateway. Its zero value means no authentication at all.
type prometheusAuth struct {
	username string
	password string
	// client carries the bearer token transport and is shared by every pusher.
	// It is nil unless a bearer token is configured.
	client *http.Client
}

// String implements fmt.Stringer with redacted credentials so the configuration
// can never leak a password or a bearer token through log or error formatting.
func (a prometheusAuth) String() string {
	switch {
	case a.client != nil:
		return "bearer token authentication"
	case a.username != "":
		return "basic authentication"
	default:
		return "no authentication"
	}
}

// apply configures pusher with the resolved credentials. Pushers without any
// configured authentication are returned untouched, which keeps unauthenticated
// push gateways working exactly as before.
func (a prometheusAuth) apply(pusher *push.Pusher) *push.Pusher {
	switch {
	case a.client != nil:
		return pusher.Client(a.client)
	case a.username != "":
		return pusher.BasicAuth(a.username, a.password)
	default:
		return pusher
	}
}

// resolveFlagOrEnv returns value if the flag was explicitly set on the command
// line, otherwise it falls back to the environment variable env.
func resolveFlagOrEnv(cmd *cobra.Command, flag, value, env string) string {
	if cmd.Flags().Changed(flag) {
		return value
	}

	if fromEnv, has := os.LookupEnv(env); has {
		return fromEnv
	}

	return value
}

// newPrometheusAuth resolves the authentication configuration from the given
// flag values and their corresponding environment variables. Explicitly set
// flags take precedence over the environment. It fails before any network
// request is made if the resulting configuration is not usable.
func newPrometheusAuth(cmd *cobra.Command, gatewayURL, username, password, bearerToken string) (prometheusAuth, error) {
	username = resolveFlagOrEnv(cmd, "prometheus-username", username, envPrometheusUsername)
	password = resolveFlagOrEnv(cmd, "prometheus-password", password, envPrometheusPassword)
	bearerToken = resolveFlagOrEnv(cmd, "prometheus-bearer-token", bearerToken, envPrometheusBearerToken)

	hasBasicAuth := username != "" || password != ""

	if hasBasicAuth && bearerToken != "" {
		return prometheusAuth{}, errors.New("prometheus basic auth and bearer token authentication are mutually exclusive, configure only one of them")
	}

	if hasBasicAuth && (username == "" || password == "") {
		return prometheusAuth{}, errors.New("prometheus basic auth requires both a username and a password")
	}

	if bearerToken != "" {
		origin, err := pushGatewayOrigin(gatewayURL)
		if err != nil {
			return prometheusAuth{}, err
		}

		return prometheusAuth{
			client: &http.Client{
				Transport: &bearerAuthRoundTripper{
					token:  bearerToken,
					origin: origin,
					rt:     http.DefaultTransport,
				},
			},
		}, nil
	}

	return prometheusAuth{username: username, password: password}, nil
}

func newPushCmd() *cobra.Command {
	var (
		prometheusArg         = "http://localhost:9091"
		job                   = "renovate"
		bufferSize            = 10485760
		logLevel              = 0
		prometheusUsername    string
		prometheusPassword    string
		prometheusBearerToken string
	)

	pushCmd := &cobra.Command{
		Use: "push",
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Resolved first so a broken authentication configuration fails
			// before any input is read or any request is sent.
			auth, err := newPrometheusAuth(cmd, prometheusArg, prometheusUsername, prometheusPassword, prometheusBearerToken)
			if err != nil {
				return err
			}

			var file *os.File

			if fileArg == "-" {
				file = os.Stdin
			} else {
				f, err := os.Open(fileArg)
				if err != nil {
					return err
				}

				defer func() { _ = f.Close() }()
				file = f
			}

			log := stdr.New(newStdLogger(log.Lshortfile))
			stdr.SetVerbosity(logLevel)

			parser := parser.NewParser(file, parser.ParserOptions{
				BufferSize: bufferSize,
				Logger:     log,
			})

			collectors, err := parser.Parse()
			if err != nil {
				return err
			}

			for repository, collector := range collectors {
				// Note: Client can't be reused, as there is no way to unregister a Collector from a Pusher.
				client := auth.apply(push.New(prometheusArg, job))
				client.Grouping("repository", repository)

				if err := client.Delete(); err != nil {
					return err
				}

				if err := client.Collector(collector).Push(); err != nil {
					return err
				}
			}

			return err
		},
	}

	pushCmd.Flags().StringVarP(&prometheusArg, "prometheus", "", prometheusArg, "Prometheus push gateway URL")
	pushCmd.Flags().StringVarP(&job, "job", "", job, "Value of job label used when pushing metrics")
	pushCmd.Flags().IntVarP(&bufferSize, "buffer-size", "", bufferSize, "Buffer size while parsing input")
	pushCmd.Flags().IntVarP(&logLevel, "log-level", "", logLevel, "Log Level (Default is 0 which is no logging)")
	pushCmd.Flags().StringVarP(&prometheusUsername, "prometheus-username", "", "", "Username used for basic authentication against the prometheus push gateway (env "+envPrometheusUsername+")")
	pushCmd.Flags().StringVarP(&prometheusPassword, "prometheus-password", "", "", "Password used for basic authentication against the prometheus push gateway (env "+envPrometheusPassword+")")
	pushCmd.Flags().StringVarP(&prometheusBearerToken, "prometheus-bearer-token", "", "", "Bearer token used for authentication against the prometheus push gateway (env "+envPrometheusBearerToken+")")

	return pushCmd
}

func init() {
	rootCmd.AddCommand(newPushCmd())
}
