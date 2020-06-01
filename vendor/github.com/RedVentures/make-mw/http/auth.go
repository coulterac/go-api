//go:generate mockgen -destination=mock/auth.go -package=mock -source=auth.go

package http

import (
	"net/http"
	"strings"

	rvAuth "github.com/RedVentures/sdk-go/auth"
)

type Verifier interface {
	VerifyToken(string) (*rvAuth.Token, error)
}

type Scopes struct {
	Verifier Verifier
}

// WithScope will be sure the passed auth token has the correct scope
func (s *Scopes) WithScope(next http.Handler, scope string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.Replace(r.Header.Get("Authorization"), "Bearer ", "", 1)

		// Check that the token is valid
		t, err := s.Verifier.VerifyToken(token)
		if err != nil {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		scopes := strings.Split(t.Claims.Scope, " ")

		// Check that the token has the scope that we are looking for
		if !contains(scopes, scope) {
			w.WriteHeader(http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func contains(haystack []string, needle string) bool {
	for _, hay := range haystack {
		if hay == needle {
			return true
		}
	}
	return false
}
