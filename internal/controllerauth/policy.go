package controllerauth

import (
	"net/http"
	"strings"
)

type Decision string

const (
	DecisionAllowed          Decision = "allowed"
	DecisionPublic           Decision = "public"
	DecisionDenied           Decision = "denied"
	DecisionMethodNotAllowed Decision = "method_not_allowed"
	DecisionNotFound         Decision = "not_found"
)

type RouteClass string

const (
	RouteUnknown   RouteClass = "unknown"
	RoutePublic    RouteClass = "public"
	RouteProtected RouteClass = "protected"
)

type Policy struct {
	rules []routeRule
}

type routeRule struct {
	method string
	route  routePattern
	public bool
	roles  []Role
}

type routePattern struct {
	exact  string
	prefix string
	suffix string
}

func ControllerPolicy() Policy {
	return Policy{rules: []routeRule{
		publicExact(http.MethodGet, "/healthz"),
		roleExact(http.MethodPost, "/workflow", RoleClient, RoleAdmin),
		roleFamily(http.MethodGet, "/workflow-runs/", "/source-bundle.zip", RoleClient, RoleWorker, RoleAdmin),
		roleFamily(http.MethodGet, "/submissions/", "/status", RoleClient, RoleAdmin),
		roleFamily(http.MethodGet, "/submissions/", "/logs", RoleClient, RoleAdmin),
		roleExact(http.MethodPost, "/work", RoleClient, RoleAdmin),
		roleExact(http.MethodGet, "/work/next", RoleWorker, RoleAdmin),
		roleExact(http.MethodPost, "/work/complete", RoleWorker, RoleAdmin),
		roleExact(http.MethodPost, "/work/fail", RoleWorker, RoleAdmin),
		roleExact(http.MethodPost, "/workers/register", RoleWorker, RoleAdmin),
		roleExact(http.MethodPost, "/workers/heartbeat", RoleWorker, RoleAdmin),
		roleExact(http.MethodPost, "/workers/stop", RoleWorker, RoleAdmin),
		roleExact(http.MethodGet, "/status", RoleClient, RoleAdmin),
		roleExact(http.MethodGet, "/observations/logs", RoleClient, RoleAdmin),
		roleExact(http.MethodPost, "/shutdown", RoleAdmin),
	}}
}

func (p Policy) Authorize(method string, path string, role Role) Decision {
	matchedPath := false
	for _, rule := range p.rules {
		if !rule.route.match(path) {
			continue
		}
		matchedPath = true
		if method != rule.method {
			continue
		}
		if rule.public {
			return DecisionPublic
		}
		for _, allowed := range rule.roles {
			if role == allowed {
				return DecisionAllowed
			}
		}
		return DecisionDenied
	}
	if matchedPath {
		return DecisionMethodNotAllowed
	}
	return DecisionNotFound
}

func (p Policy) Classify(path string) RouteClass {
	class := RouteUnknown
	for _, rule := range p.rules {
		if !rule.route.match(path) {
			continue
		}
		if rule.public {
			return RoutePublic
		}
		class = RouteProtected
	}
	return class
}

func publicExact(method string, path string) routeRule {
	return routeRule{
		method: method,
		route:  routePattern{exact: path},
		public: true,
	}
}

func roleExact(method string, path string, roles ...Role) routeRule {
	return routeRule{
		method: method,
		route:  routePattern{exact: path},
		roles:  roles,
	}
}

func roleFamily(method string, prefix string, suffix string, roles ...Role) routeRule {
	return routeRule{
		method: method,
		route: routePattern{
			prefix: prefix,
			suffix: suffix,
		},
		roles: roles,
	}
}

func (p routePattern) match(path string) bool {
	if p.exact != "" {
		return path == p.exact
	}
	if !strings.HasPrefix(path, p.prefix) || !strings.HasSuffix(path, p.suffix) {
		return false
	}
	item := strings.TrimPrefix(path, p.prefix)
	item = strings.TrimSuffix(item, p.suffix)
	return item != "" && !strings.Contains(item, "/")
}
