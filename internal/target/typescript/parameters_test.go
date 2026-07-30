package typescript

import "testing"

func TestClassifyFetchRequestHeader(t *testing.T) {
	t.Parallel()

	environmentControlled := []string{
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
	for _, name := range environmentControlled {
		if got := classifyFetchRequestHeader(name); got != requestHeaderEnvironmentControlled {
			t.Errorf("classifyFetchRequestHeader(%q) = %v, want environment controlled", name, got)
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
		"x-http-method",
		"X-HTTP-Method-Override",
		"x-method-override",
		"x-http-methods",
	}
	for _, name := range callerManaged {
		if got := classifyFetchRequestHeader(name); got != requestHeaderCallerManaged {
			t.Errorf("classifyFetchRequestHeader(%q) = %v, want caller managed", name, got)
		}
	}
}

func TestProjectClientParameterOwnsEnvironmentRequiredness(t *testing.T) {
	t.Parallel()

	environment := operationParameter{Required: true, EnvironmentControlled: true}
	if got := projectClientParameter(environment); got.Required {
		t.Fatal("environment-controlled client parameter remained required")
	}
	if !environment.Required {
		t.Fatal("client projection mutated the OpenAPI parameter")
	}

	caller := operationParameter{Required: true}
	if got := projectClientParameter(caller); !got.Required {
		t.Fatal("caller-controlled client parameter lost requiredness")
	}
}
