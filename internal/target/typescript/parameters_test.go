package typescript

import "testing"

func TestClassifyFetchRequestHeader(t *testing.T) {
	t.Parallel()

	hostManaged := []string{
		"accept-charset",
		"accept-encoding",
		"access-control-request-headers",
		"access-control-request-method",
		"connection",
		"content-length",
		"cookie",
		"cookie2",
		"date",
		"dnt",
		"expect",
		"host",
		"keep-alive",
		"origin",
		"referer",
		"set-cookie",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade",
		"via",
		"Origin",
		"oRiGiN",
		"Proxy-Authorization",
		"proxy-custom",
		"Sec-Fetch-Site",
		"SEC-CUSTOM",
	}
	for _, name := range hostManaged {
		if got := classifyFetchRequestHeader(name); got != requestHeaderHostManaged {
			t.Errorf("classifyFetchRequestHeader(%q) = %v, want host managed", name, got)
		}
	}

	conditional := []string{
		"x-http-method",
		"X-HTTP-Method-Override",
		"x-method-override",
	}
	for _, name := range conditional {
		if got := classifyFetchRequestHeader(name); got != requestHeaderForbiddenMethodValue {
			t.Errorf("classifyFetchRequestHeader(%q) = %v, want forbidden method value", name, got)
		}
	}

	callerManaged := []string{
		"authorization",
		"content-type",
		"user-agent",
		"User-Agent",
		"x-origin",
		"proxy",
		"security",
		"x-http-methods",
	}
	for _, name := range callerManaged {
		if got := classifyFetchRequestHeader(name); got != requestHeaderCallerManaged {
			t.Errorf("classifyFetchRequestHeader(%q) = %v, want caller managed", name, got)
		}
	}
}
