package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

// A single renovate log line which makes the parser emit one repository and
// therefore triggers exactly one Delete and one Push request.
const testLogLine = `{"msg":"packageFiles with updates","repository":"acme/repo","baseBranch":"main","config":{"dockerfile":[{"packageFile":"Dockerfile","deps":[{"depName":"example","packageName":"ghcr.io/acme/example","currentValue":"1.0.0","updates":[{"newVersion":"2.0.0","updateType":"major","releaseTimestamp":"2026-06-01T00:00:00.000Z"}]}]}]}}`

type capturedRequest struct {
	method     string
	authHeader string
	username   string
	password   string
	hasBasic   bool
}

// pushgateway is a stub of the prometheus push gateway which records every
// request it receives together with its authentication data.
type pushgateway struct {
	*httptest.Server
	mu       sync.Mutex
	requests []capturedRequest
}

func newPushgateway(t *testing.T) *pushgateway {
	t.Helper()

	gateway := &pushgateway{}
	gateway.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, hasBasic := r.BasicAuth()

		gateway.mu.Lock()
		gateway.requests = append(gateway.requests, capturedRequest{
			method:     r.Method,
			authHeader: r.Header.Get("Authorization"),
			username:   username,
			password:   password,
			hasBasic:   hasBasic,
		})
		gateway.mu.Unlock()

		// Both Delete() and Push() of the prometheus client accept 202.
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(gateway.Close)

	return gateway
}

func (g *pushgateway) captured() []capturedRequest {
	g.mu.Lock()
	defer g.mu.Unlock()

	return append([]capturedRequest(nil), g.requests...)
}

// assertDeleteAndPush fails unless the gateway saw exactly one Delete followed
// by one Push request.
func (g *pushgateway) assertDeleteAndPush(t *testing.T) []capturedRequest {
	t.Helper()

	requests := g.captured()
	if len(requests) != 2 {
		t.Fatalf("got %d requests, want 2: %+v", len(requests), requests)
	}
	if requests[0].method != http.MethodDelete {
		t.Errorf("first request method = %q, want %q", requests[0].method, http.MethodDelete)
	}
	if requests[1].method != http.MethodPut {
		t.Errorf("second request method = %q, want %q", requests[1].method, http.MethodPut)
	}

	return requests
}

// unsetEnv removes the given environment variables for the duration of the test.
// t.Setenv registers the restore of the original value, os.Unsetenv then makes
// the variable absent so the fallback behaves as on a clean environment.
func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
	}
}

// cleanAuthEnv removes every authentication environment variable so a developer
// environment can not influence the test result.
func cleanAuthEnv(t *testing.T) {
	t.Helper()
	unsetEnv(t, envPrometheusUsername, envPrometheusPassword, envPrometheusBearerToken)
}

// useTestLogFile points the global file argument at a log file holding a single
// parsable renovate line.
func useTestLogFile(t *testing.T) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "renovate.log")
	if err := os.WriteFile(path, []byte(testLogLine+"\n"), 0o600); err != nil {
		t.Fatalf("write log file: %v", err)
	}

	previous := fileArg
	fileArg = path
	t.Cleanup(func() { fileArg = previous })
}

// runPush executes a fresh push command so neither flag values nor their
// "changed" state leak between test cases.
func runPush(t *testing.T, args ...string) error {
	t.Helper()

	cmd := newPushCmd()
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)

	return cmd.Execute()
}

// Without any credentials the push gateway must be contacted exactly as before,
// which means no Authorization header at all.
func TestPush_NoAuthentication(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	if err := runPush(t, "--prometheus="+gateway.URL); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.authHeader != "" {
			t.Errorf("%s request has Authorization header %q, want none", request.method, request.authHeader)
		}
	}
}

// Basic auth credentials from the environment must be attached to the Delete as
// well as to the Push request.
func TestPush_BasicAuthFromEnvironment(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusUsername, "env-user")
	t.Setenv(envPrometheusPassword, "env-secret")

	if err := runPush(t, "--prometheus="+gateway.URL); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if !request.hasBasic {
			t.Errorf("%s request has no basic auth, header = %q", request.method, request.authHeader)
			continue
		}
		if request.username != "env-user" || request.password != "env-secret" {
			t.Errorf("%s request credentials = %q/%q, want %q/%q", request.method, request.username, request.password, "env-user", "env-secret")
		}
	}
}

// The same must work when the credentials are passed as flags.
func TestPush_BasicAuthFromFlags(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	if err := runPush(t,
		"--prometheus="+gateway.URL,
		"--prometheus-username=flag-user",
		"--prometheus-password=flag-secret",
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.username != "flag-user" || request.password != "flag-secret" {
			t.Errorf("%s request credentials = %q/%q, want %q/%q", request.method, request.username, request.password, "flag-user", "flag-secret")
		}
	}
}

// A bearer token from the environment must be attached to both requests.
func TestPush_BearerTokenFromEnvironment(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusBearerToken, "env-token")

	if err := runPush(t, "--prometheus="+gateway.URL); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.authHeader != "Bearer env-token" {
			t.Errorf("%s request Authorization = %q, want %q", request.method, request.authHeader, "Bearer env-token")
		}
	}
}

// The same must work when the token is passed as a flag.
func TestPush_BearerTokenFromFlag(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	if err := runPush(t, "--prometheus="+gateway.URL, "--prometheus-bearer-token=flag-token"); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.authHeader != "Bearer flag-token" {
			t.Errorf("%s request Authorization = %q, want %q", request.method, request.authHeader, "Bearer flag-token")
		}
	}
}

// Explicitly set flags win over the environment.
func TestPush_BasicAuthFlagsOverrideEnvironment(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusUsername, "env-user")
	t.Setenv(envPrometheusPassword, "env-secret")

	if err := runPush(t,
		"--prometheus="+gateway.URL,
		"--prometheus-username=flag-user",
		"--prometheus-password=flag-secret",
	); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.username != "flag-user" || request.password != "flag-secret" {
			t.Errorf("%s request credentials = %q/%q, want %q/%q", request.method, request.username, request.password, "flag-user", "flag-secret")
		}
	}
}

// The bearer token flag also wins over its environment variable.
func TestPush_BearerTokenFlagOverridesEnvironment(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusBearerToken, "env-token")

	if err := runPush(t, "--prometheus="+gateway.URL, "--prometheus-bearer-token=flag-token"); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.authHeader != "Bearer flag-token" {
			t.Errorf("%s request Authorization = %q, want %q", request.method, request.authHeader, "Bearer flag-token")
		}
	}
}

// An empty flag value is an explicit value as well and therefore disables the
// credential coming from the environment.
func TestPush_EmptyFlagOverridesEnvironment(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusBearerToken, "env-token")

	if err := runPush(t, "--prometheus="+gateway.URL, "--prometheus-bearer-token="); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range gateway.assertDeleteAndPush(t) {
		if request.authHeader != "" {
			t.Errorf("%s request has Authorization header %q, want none", request.method, request.authHeader)
		}
	}
}

// Half configured basic auth must fail before anything is sent over the network.
func TestPush_IncompleteBasicAuthFails(t *testing.T) {
	for name, args := range map[string][]string{
		"username only": {"--prometheus-username=only-user"},
		"password only": {"--prometheus-password=only-secret"},
	} {
		t.Run(name, func(t *testing.T) {
			cleanAuthEnv(t)
			useTestLogFile(t)
			gateway := newPushgateway(t)

			err := runPush(t, append([]string{"--prometheus=" + gateway.URL}, args...)...)
			if err == nil {
				t.Fatal("push succeeded, want a configuration error")
			}
			if !strings.Contains(err.Error(), "requires both a username and a password") {
				t.Errorf("error = %q, want it to mention the missing credential", err)
			}
			if requests := gateway.captured(); len(requests) != 0 {
				t.Errorf("got %d requests, want none: %+v", len(requests), requests)
			}
			if strings.Contains(err.Error(), "only-secret") {
				t.Errorf("error %q leaks the password", err)
			}
		})
	}
}

// The same applies when the environment provides only one half of the credentials.
func TestPush_IncompleteBasicAuthFromEnvironmentFails(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusUsername, "env-user")

	err := runPush(t, "--prometheus="+gateway.URL)
	if err == nil {
		t.Fatal("push succeeded, want a configuration error")
	}
	if requests := gateway.captured(); len(requests) != 0 {
		t.Errorf("got %d requests, want none: %+v", len(requests), requests)
	}
}

// Basic auth and bearer token authentication are mutually exclusive and must be
// rejected before any request is sent.
func TestPush_BasicAuthAndBearerTokenFails(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	err := runPush(t,
		"--prometheus="+gateway.URL,
		"--prometheus-username=some-user",
		"--prometheus-password=some-secret",
		"--prometheus-bearer-token=some-token",
	)
	if err == nil {
		t.Fatal("push succeeded, want a configuration error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want it to mention that both are mutually exclusive", err)
	}
	if requests := gateway.captured(); len(requests) != 0 {
		t.Errorf("got %d requests, want none: %+v", len(requests), requests)
	}
	for _, secret := range []string{"some-secret", "some-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q leaks %q", err, secret)
		}
	}
}

// Mixing environment and flags must be detected as well.
func TestPush_BasicAuthFromEnvironmentAndBearerTokenFlagFails(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)
	gateway := newPushgateway(t)

	t.Setenv(envPrometheusUsername, "env-user")
	t.Setenv(envPrometheusPassword, "env-secret")

	err := runPush(t, "--prometheus="+gateway.URL, "--prometheus-bearer-token=flag-token")
	if err == nil {
		t.Fatal("push succeeded, want a configuration error")
	}
	if requests := gateway.captured(); len(requests) != 0 {
		t.Errorf("got %d requests, want none: %+v", len(requests), requests)
	}
	for _, secret := range []string{"env-secret", "flag-token"} {
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error %q leaks %q", err, secret)
		}
	}
}

// Secrets must not show up in the generated command help.
func TestPushCmd_HelpHasNoSecrets(t *testing.T) {
	cleanAuthEnv(t)

	t.Setenv(envPrometheusUsername, "help-user")
	t.Setenv(envPrometheusPassword, "help-secret")
	t.Setenv(envPrometheusBearerToken, "help-token")

	usage := newPushCmd().UsageString()
	for _, secret := range []string{"help-user", "help-secret", "help-token"} {
		if strings.Contains(usage, secret) {
			t.Errorf("usage leaks %q:\n%s", secret, usage)
		}
	}
}

// Formatting the configuration must never render a credential.
func TestPrometheusAuth_StringRedactsCredentials(t *testing.T) {
	cleanAuthEnv(t)

	for name, test := range map[string]struct {
		args    []string
		want    string
		secrets []string
	}{
		"none": {
			want: "no authentication",
		},
		"basic": {
			args:    []string{"--prometheus-username=a-user", "--prometheus-password=s3cr3t-value"},
			want:    "basic authentication",
			secrets: []string{"s3cr3t-value"},
		},
		"bearer": {
			args:    []string{"--prometheus-bearer-token=t0ken-value"},
			want:    "bearer token authentication",
			secrets: []string{"t0ken-value"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var auth prometheusAuth

			cmd := newPushCmd()
			cmd.SilenceUsage = true
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			cmd.RunE = func(cmd *cobra.Command, _ []string) error {
				var err error
				auth, err = newPrometheusAuth(cmd,
					cmd.Flag("prometheus").Value.String(),
					cmd.Flag("prometheus-username").Value.String(),
					cmd.Flag("prometheus-password").Value.String(),
					cmd.Flag("prometheus-bearer-token").Value.String(),
				)

				return err
			}
			cmd.SetArgs(test.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("resolve auth: %v", err)
			}

			if got := auth.String(); got != test.want {
				t.Errorf("String() = %q, want %q", got, test.want)
			}
			for _, secret := range test.secrets {
				if strings.Contains(auth.String(), secret) {
					t.Errorf("String() = %q leaks %q", auth.String(), secret)
				}
			}
		})
	}
}

// The bearer transport must not modify the request it is handed and must use the
// transport it wraps.
func TestBearerAuthRoundTripper_ClonesRequestAndKeepsTransport(t *testing.T) {
	inner := &recordingRoundTripper{}
	transport := &bearerAuthRoundTripper{token: "a-token", origin: "http://example.com", rt: inner}

	request, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	if _, err := transport.RoundTrip(request); err != nil {
		t.Fatalf("round trip: %v", err)
	}

	if got := request.Header.Get("Authorization"); got != "" {
		t.Errorf("original request was mutated, Authorization = %q", got)
	}
	if inner.request == nil {
		t.Fatal("wrapped transport was not used")
	}
	if got := inner.request.Header.Get("Authorization"); got != "Bearer a-token" {
		t.Errorf("forwarded Authorization = %q, want %q", got, "Bearer a-token")
	}
}

// A round tripper runs once per redirect hop, so it must only attach the token
// to requests aimed at the configured push gateway. Otherwise a redirecting
// gateway could hand the credential to a third party or to a plain http target.
func TestBearerAuthRoundTripper_OnlyAuthenticatesTheConfiguredOrigin(t *testing.T) {
	for name, test := range map[string]struct {
		target string
		want   string
	}{
		"same origin":       {target: "https://gateway.example:9091/metrics/job/renovate", want: "Bearer a-token"},
		"same origin, root": {target: "https://gateway.example:9091/", want: "Bearer a-token"},
		"other host":        {target: "https://evil.example:9091/metrics", want: ""},
		"other port":        {target: "https://gateway.example:9092/metrics", want: ""},
		"scheme downgrade":  {target: "http://gateway.example:9091/metrics", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			inner := &recordingRoundTripper{}
			transport := &bearerAuthRoundTripper{
				token:  "a-token",
				origin: "https://gateway.example:9091",
				rt:     inner,
			}

			request, err := http.NewRequest(http.MethodGet, test.target, nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}

			if _, err := transport.RoundTrip(request); err != nil {
				t.Fatalf("round trip: %v", err)
			}

			if inner.request == nil {
				t.Fatal("wrapped transport was not used")
			}
			if got := inner.request.Header.Get("Authorization"); got != test.want {
				t.Errorf("Authorization for %s = %q, want %q", test.target, got, test.want)
			}
		})
	}
}

// End to end: a push gateway that redirects elsewhere must not leak the token to
// the redirect target.
func TestPush_BearerTokenIsNotLeakedToRedirectTarget(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)

	thirdParty := newPushgateway(t)
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, thirdParty.URL+r.URL.Path, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(gateway.Close)

	if err := runPush(t, "--prometheus="+gateway.URL, "--prometheus-bearer-token=secret-token"); err != nil {
		t.Fatalf("push: %v", err)
	}

	for _, request := range thirdParty.assertDeleteAndPush(t) {
		if request.authHeader != "" {
			t.Errorf("redirect target received Authorization %q for the %s request, want none", request.authHeader, request.method)
		}
	}
}

// A malformed push gateway URL must be rejected before any request is sent, and
// the error must not echo the URL back because it may carry credentials.
func TestPush_BearerTokenWithUnusableGatewayURLFails(t *testing.T) {
	cleanAuthEnv(t)
	useTestLogFile(t)

	err := runPush(t, "--prometheus=:", "--prometheus-bearer-token=some-token")
	if err == nil {
		t.Fatal("push succeeded, want a configuration error")
	}
	if strings.Contains(err.Error(), "some-token") {
		t.Errorf("error %q leaks the token", err)
	}
}

type recordingRoundTripper struct {
	request *http.Request
}

func (rt *recordingRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.request = req

	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}
