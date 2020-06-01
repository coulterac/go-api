package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// defaultHTTPClient is the default HTTP client used when one isn't provided.
var defaultHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
}

// Granter is used to grant permission to access-protected resources. ClientID, ClientSecret, and
// TenantURL fields MUST BE set for it to work.
//
// A single granter can grant access to multiple protected resources. All granter methods are
// guaranteed to be thread-safe as long as none of the public fields are modified after a granter
// method is called.
type Granter struct {
	// Client ID is the OAuth client ID for the current service. Granter won't work without this.
	ClientID string

	// ClientSecret is the OAuth client secret for the current service. Granter won't work without
	// this.
	ClientSecret string

	// TenantURL is the Auth0 tenant URL. Granter won't work without this. It follows this
	// convention: "https://TENANTNAME.auth0.com".
	TenantURL string

	// HTTPClient defines the HTTP client used to request the token. If one isn't provided
	// defaultHTTPClient is used.
	HTTPClient *http.Client

	// ExpirationMargin defines the buffer of time between when the cache expires and a JWT expires. This setting
	// prevents the cache from expiring before it is verified by the other
	// service.
	ExpirationMargin int64

	cache             map[string]cachedToken
	mutex             sync.RWMutex
	tokenRequestGroup singleflight.Group
}

// GetToken gets a JWT from the cache for the requested audience.
//
// If nothing exists in the cache or the cached token has expired, a new token is fetched from the
// OAuth token service.
func (g *Granter) GetToken(resource string) (jwt string, err error) {
	// If resource is an empty string than none of this is going to matter so bail with an error
	if resource == "" {
		return jwt, errors.New("resource cannot be empty")
	}

	// do we already have the token in the cache?
	if jwt, ok := g.readToken(resource); ok {
		return jwt, nil
	}

	// Ensure that we don't end up with simulataneous requests for a particular token. Since it is
	// keyed by the resource, simultaneous requests for different tokens will still work properly
	token, err, _ := g.tokenRequestGroup.Do(resource, func() (token interface{}, err error) {
		// We should get an error from Auth0 if ClientID, ClientSecret, or Resource are invalid, but
		// since we know it won't if any of them are empty let's check for them here instead of
		// wasting time sending a bad request. We already checked resource at the top of this
		// function so we don't need to check that again.
		if g.ClientID == "" || g.ClientSecret == "" {
			return token, errors.New("ClientID and ClientSecret cannot be empty")
		}

		if g.TenantURL == "" {
			return token, errors.New("TenantURL cannot be empty")
		}

		// Use the default client if one isn't provided to prevent runtime errors. Since a client
		// should be passed in we'll default to that, so we'll only need to override it when it's
		// not provided.
		client := g.HTTPClient
		if client == nil {
			client = defaultHTTPClient
		}

		// We can ignore the error since we are using a fixed type with all string fields. It shouldn't
		// be possible to get an error here. If something does slip by, then it we will get an error
		// when we get a response from Auth0
		payload, _ := json.Marshal(map[string]string{
			"grant_type":    "client_credentials",
			"client_id":     g.ClientID,
			"client_secret": g.ClientSecret,
			"audience":      resource,
		})

		// Remove trailing slashes if present.
		tenantURL := strings.TrimRight(g.TenantURL, "/")

		resp, err := client.Post(tenantURL+"/oauth/token", "application/json", bytes.NewBuffer(payload))
		if err != nil {
			return "", errors.Wrap(err, "unable to fetch token")
		}

		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			err = fmt.Errorf("received %d status code", resp.StatusCode)
			return "", errors.Wrap(err, "unable to fetch token")
		}

		var accessTokenResponse struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   int64  `json:"expires_in"`
		}

		err = json.NewDecoder(resp.Body).Decode(&accessTokenResponse)
		if err != nil {
			return "", errors.Wrap(err, "bad Access Token Response")
		}

		// get the expiration of the token in unix time
		expiresOn := time.Now().Unix() + accessTokenResponse.ExpiresIn

		// save the token to the cache
		g.writeToken(resource, accessTokenResponse.AccessToken, expiresOn)

		return accessTokenResponse.AccessToken, nil
	})

	if err != nil {
		return
	}

	// singleFlight only gives us an interface so we've got to assert it to a string
	return token.(string), nil

}

// NewTokenFunc creates a function that gets a token for a particular resource to aid in dependency
// injection. This allows you to pass down only the function instead of having to pass down a
// granter and a resource string.
func (g *Granter) NewTokenFunc(resource string) func() (jwt string, err error) {
	return func() (jwt string, err error) {
		return g.GetToken(resource)
	}
}

// NewRequestFunc creates a new request function for the given resource. It returns a new request that includes a method,
// URL, optional body, and a set Auth header.
//
// This is the preferred way of using this library so you don't have to worry about holding on to
// tokens or setting headers. The returned function wraps
// http.NewRequest(https://golang.org/pkg/net/http/#NewRequest) adding the Authorization header. If
// a valid token exists in the cache it is used. Otherwise, a new token is fetched.
func (g *Granter) NewRequestFunc(resource string) func(method, url string, body io.Reader) (*http.Request, error) {
	return func(method, url string, body io.Reader) (r *http.Request, err error) {
		// get jwt
		jwt, err := g.GetToken(resource)
		if err != nil {
			return
		}

		r, err = http.NewRequest(method, url, body)
		if err != nil {
			return
		}

		r.Header.Add("Authorization", "Bearer "+jwt)

		return
	}
}

// ResetCache clears the cached tokens for all of the resources on this granter.
func (g *Granter) ResetCache() {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	g.cache = nil
}

// cachedToken defines how cached JWTs are stored in the cache.
type cachedToken struct {
	jwt        string
	expiration int64
}

// readToken reads the token from the tokenCache store, ensures that the token exists in the cache,
// and is not expired.
func (g *Granter) readToken(resource string) (jwt string, ok bool) {
	g.mutex.RLock()
	defer g.mutex.RUnlock()

	// the cache is completely empty so no need to even check for the presence of the key
	if g.cache == nil {
		return
	}

	// ensure we have a cache and it hasn't expired yet
	if tc, ok := g.cache[resource]; ok && tc.expiration >= time.Now().Unix() {
		return tc.jwt, true
	}

	return
}

// writeToken updates the tokenCache with the given token and expiration (in seconds). The expiration is adjusted
// by the expiration margin.
func (g *Granter) writeToken(resource string, jwt string, expiration int64) {
	g.mutex.Lock()
	defer g.mutex.Unlock()

	// make sure cache has already been made
	if g.cache == nil {
		g.cache = make(map[string]cachedToken)
	}

	// set the cache we want to write
	g.cache[resource] = cachedToken{
		jwt:        jwt,
		expiration: expiration - g.ExpirationMargin,
	}
}

// NewRoundTripper creates an http.RoundTripper that adds authorization to each request.
//
// The http.RoundTripper returned will add a token for the given resource to the request as an authorization header
// before delegating to the original RoundTripper provided (or http.DefaultTransport if none is provided). If granter is
// nil, NewRoundTripper will panic.
//
// Example use:
//
//   granter := &auth.Granter{}
//   client := &http.Client{}
//   client.Transport = auth.NewRoundTripper(granter, "https://cyberdyne-robot.com", client.Transport)
//   request, _ := http.NewRequest("GET", "http://example.com", nil)
//   resp, err := client.Do(request)
//
func NewRoundTripper(granter *Granter, resource string, original http.RoundTripper) http.RoundTripper {
	if granter == nil {
		panic("granter cannot be nil")
	}

	return roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		jwt, err := granter.GetToken(resource)
		if err != nil {
			return nil, err
		}
		request.Header.Add("Authorization", "Bearer "+jwt)

		if original == nil {
			return http.DefaultTransport.RoundTrip(request)
		}
		return original.RoundTrip(request)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
