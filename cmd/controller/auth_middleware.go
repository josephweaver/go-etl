package main

import (
	"context"
	"net/http"
	"strings"

	"goetl/internal/controllerauth"
)

type controllerPrincipalContextKey struct{}

func principalFromContext(ctx context.Context) (controllerauth.Principal, bool) {
	principal, ok := ctx.Value(controllerPrincipalContextKey{}).(controllerauth.Principal)
	return principal, ok
}

func withPrincipal(ctx context.Context, principal controllerauth.Principal) context.Context {
	return context.WithValue(ctx, controllerPrincipalContextKey{}, principal)
}

func (c *Controller) authorizeControllerRoutes(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c.authMode == controllerauth.ModeDisabled {
			next.ServeHTTP(w, r)
			return
		}

		switch c.authPolicy.Classify(r.URL.Path) {
		case controllerauth.RouteUnknown:
			http.NotFound(w, r)
			return
		case controllerauth.RoutePublic:
			switch c.authPolicy.Authorize(r.Method, r.URL.Path, "") {
			case controllerauth.DecisionPublic:
				next.ServeHTTP(w, r)
			case controllerauth.DecisionMethodNotAllowed:
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			default:
				http.NotFound(w, r)
			}
			return
		}

		token, ok := bearerToken(r.Header)
		if !ok {
			writeAuthenticationFailure(w, http.StatusUnauthorized)
			return
		}
		principal, ok := c.authStore.MatchBearer(token)
		if !ok {
			writeAuthenticationFailure(w, http.StatusUnauthorized)
			return
		}

		switch c.authPolicy.Authorize(r.Method, r.URL.Path, principal.Role) {
		case controllerauth.DecisionAllowed:
			next.ServeHTTP(w, r.WithContext(withPrincipal(r.Context(), principal)))
		case controllerauth.DecisionMethodNotAllowed:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		case controllerauth.DecisionNotFound:
			http.NotFound(w, r)
		default:
			writeAuthenticationFailure(w, http.StatusForbidden)
		}
	})
}

func bearerToken(header http.Header) (string, bool) {
	values := header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	value := values[0]
	if strings.Contains(value, ",") {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(value, prefix) {
		return "", false
	}
	token := strings.TrimPrefix(value, prefix)
	if token == "" || strings.ContainsAny(token, " \t\r\n") {
		return "", false
	}
	return token, true
}

func writeAuthenticationFailure(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(status), status)
}
