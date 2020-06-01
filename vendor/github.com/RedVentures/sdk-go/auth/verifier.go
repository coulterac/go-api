package auth

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/pkg/errors"
	"golang.org/x/sync/singleflight"
)

// Verifier handles verifying incoming requests to a protected service. All verifier methods are
// guaranteed to be thread-safe as long as none of the public fields are modified once any verifier
// method has been called.
type Verifier struct {
	// Resource is the resource URI for this service. Verifier won't work without this.
	Resource string

	// TenantURL is the Auth0 tenant URL. Granter won't work without this. It follows this
	// convention: "https://TENANTNAME.auth0.com".
	TenantURL string

	// HTTPClient is the http client used to request the token. If one isn't provided
	// defaultHTTPClient will be used.
	HTTPClient *http.Client

	// ExpirationMargin gives a buffer of time between when the cache expires and a JWT expires to
	// prevent expiration between when it's requested and when it's verified by the other
	// service.
	ExpirationMargin int64

	cache        map[string]keyCache
	mutex        sync.RWMutex
	requestGroup singleflight.Group
}

type keyCache struct {
	key        *rsa.PublicKey
	expiration int64
}

// Claims represents the claims for a JWT
type Claims struct {
	Scope      string       `json:"scope"`
	Audience   AudienceList `json:"aud,omitempty"`
	Email      string       `json:"https://email,omitempty"`
	EmployeeId string       `json:"https://employeeId,omitempty"`
	FirstName  string       `json:"https://firstName,omitempty"`
	LastName   string       `json:"https://lastName,omitempty"`

	jwt.StandardClaims
}

// Token represents a parsed JWT token
type Token struct {
	Raw    string
	Claims *Claims

	rawClaims map[string]interface{}
}

// AudienceList is meant to handle the when the special case where a JWT has one audience, the "aud" value MAY be a
// single case-sensitive string. See https://tools.ietf.org/html/rfc7519#section-4.1.3. jwt-go v4 should make this
// unnecessary - https://github.com/dgrijalva/jwt-go/issues/290.
type AudienceList []string

// RawClaims returns every single claim from the token as a map of interfaces.
// If you don't need custom claims, then you probably want to use token.Claims
// instead since it has explicit fields and types and won't require any type
// assertions on your part.
func (t *Token) RawClaims() (claims map[string]interface{}, err error) {
	if t.rawClaims != nil {
		return t.rawClaims, nil
	}

	parsed, _, err := new(jwt.Parser).ParseUnverified(t.Raw, jwt.MapClaims{})
	if err != nil {
		return
	}

	claims = make(map[string]interface{})
	for key, claim := range parsed.Claims.(jwt.MapClaims) {
		claims[key] = claim
	}

	t.rawClaims = claims

	return
}

// VerifyToken parses the given token string and verifies that it has permission to access this
// resource. An error is returned if a token has an invalid signature or does not have the correct
// permissions.
//
// In order to have permission to access this service the audience claim must match the resource URI of this
// service and the tenant ID must match the tenant of this service.
func (v *Verifier) VerifyToken(tokenString string) (token *Token, err error) {
	parsed, err := jwt.ParseWithClaims(tokenString, &Claims{}, v.keyFunc)
	if err != nil {
		return
	}

	claims, ok := parsed.Claims.(*Claims)
	if !ok {
		return nil, errors.New("unable to parse claims")
	}

	token = &Token{
		Raw:    parsed.Raw,
		Claims: claims,
	}

	return
}

// ResetCache clears the cache that storing public keys for the Verifier
func (v *Verifier) ResetCache() {
	v.mutex.Lock()
	defer v.mutex.Unlock()

	v.cache = nil
}

func (v *Verifier) keyFunc(token *jwt.Token) (interface{}, error) {
	// we need to type assert from the jwt.Claims interface to our custom claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("unable to parse claims")
	}

	// verify Audience/ResourceIdentifier
	if err := v.verifyAudience(claims.Audience); err != nil {
		return nil, err
	}

	// Verify the issuer claim. We need to add a trailing slash to the tenant URL since that's what
	// Auth0 does. However, we need to make sure that the issuer only has one trailing slash so we
	// strip any from the tenantURL to be safe.
	issuer := strings.TrimRight(v.TenantURL, "/") + "/"
	if claims.Issuer == "" || claims.Issuer != issuer {
		return nil, fmt.Errorf("bad token: issuer is '%s' when it should be '%s'", claims.Issuer, issuer)
	}

	// get public key for this kid
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, errors.New("unable to get kid header from token")
	}

	key, err := v.getKey(kid)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get public key")
	}

	return key, nil
}

// getKey gets the public key that Auth0 uses to sign tokens
func (v *Verifier) getKey(kid string) (key *rsa.PublicKey, err error) {
	// check cache
	if key, ok := v.readPublicKey(kid); ok {
		return key, nil
	}

	// Ensure that we don't end up with simulataneous requests for a particular key. Since it is
	// keyed by the kid, simultaneous requests for different kids will still work properly
	publicKey, err, _ := v.requestGroup.Do(kid, func() (publicKey interface{}, err error) {

		// Build the key url from the provided tenant url, removing any uneccesary trailing slashes.
		keyURL := strings.TrimRight(v.TenantURL, "/") + "/.well-known/jwks.json"

		// Use the default client if one isn't provided to prevent runtime errors. Since a client
		// should be passed in we'll default to that, so we'll only need to override it when it's
		// not provided.
		client := v.HTTPClient
		if client == nil {
			client = defaultHTTPClient
		}

		resp, err := client.Get(keyURL)
		if err != nil {
			return "", errors.Wrap(err, "error fetching keys from Auth0")
		}

		defer resp.Body.Close()

		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusBadRequest {
			return "", fmt.Errorf("error fetching keys from Auth0: received %d status code", resp.StatusCode)
		}

		var body struct {
			Keys []struct {
				KeyID            string   `json:"kid"`
				CertificateChain []string `json:"x5c"`
			} `json:"keys"`
		}

		err = json.NewDecoder(resp.Body).Decode(&body)
		if err != nil {
			return
		}

		// get the cert from the certificate url
		for _, key := range body.Keys {
			if key.KeyID == kid {
				if len(key.CertificateChain) == 0 {
					return nil, errors.New("missing certificate chain")
				}
				// grab the cert we want
				certString := key.CertificateChain[0]
				// put into pem format
				certString = "-----BEGIN CERTIFICATE-----\n" + certString + "\n-----END CERTIFICATE-----"
				key, err := jwt.ParseRSAPublicKeyFromPEM([]byte(certString))
				if err != nil {
					return nil, errors.Wrap(err, "unable to parse public key")
				}

				// update the keyCache with the newly acquired cert
				v.writePublicKey(kid, key)
				return key, nil
			}
		}

		return nil, errors.New("no key for kid: " + kid)
	})

	if err != nil {
		return
	}

	// singleFlight only returns an interface so we've got to assert it to a string
	return publicKey.(*rsa.PublicKey), nil

}

// readPublicKey reads the key from the keyCache store and ensures that the key exists in cache and
// is not expired
func (v *Verifier) readPublicKey(kid string) (pk *rsa.PublicKey, ok bool) {
	v.mutex.RLock()
	defer v.mutex.RUnlock()

	// if the cache is empty there is no need to actually check the key
	if v.cache == nil {
		return
	}

	// ensure we have a cache and it hasn't expired yet
	if cache, ok := v.cache[kid]; ok && cache.expiration > time.Now().Unix() {
		return cache.key, true
	}

	return
}

// writePublicKey updates the cache with a new public key
func (v *Verifier) writePublicKey(kid string, pk *rsa.PublicKey) {
	// use mutex for ordered writes
	v.mutex.Lock()
	defer v.mutex.Unlock()

	// if necessary, initialize the cache
	if v.cache == nil {
		v.cache = make(map[string]keyCache)
	}

	// set the cache we want to write
	v.cache[kid] = keyCache{
		key:        pk,
		expiration: time.Now().Unix() + 86400 - v.ExpirationMargin,
	}
}

func (v *Verifier) verifyAudience(audiences []string) error {
	for _, audience := range audiences {
		if audience == v.Resource {
			return nil
		}
	}

	return fmt.Errorf("bad token: missing '%s' audience", v.Resource)
}

func (al *AudienceList) MarshalJSON() ([]byte, error) {
	if len(*al) == 1 {
		return json.Marshal(([]string)(*al)[0])
	}
	return json.Marshal(*al)
}

func (al *AudienceList) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		var v []string
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		*al = v
		return nil
	}
	*al = []string{s}
	return nil
}
