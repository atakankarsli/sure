package jev

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestLongContextHeaderAndWireFormat(t *testing.T) {
	var headers []string
	s := serve(t, func(w http.ResponseWriter, r *http.Request) {
		headers = append(headers, r.Header.Get(LongContextHeader))
		io.WriteString(w, goodResponse)
	})

	long, err := New(Config{Transport: TransportTypeSafe, BaseURL: s.URL, APIKey: testKey, AllowInsecureBaseURL: true, LongContext: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := long.Evaluate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	plain := newTestProvider(t, TransportTypeSafe, "", s.URL, nil)
	if _, err := plain.Evaluate(context.Background(), testRequest()); err != nil {
		t.Fatal(err)
	}
	if len(headers) != 2 || headers[0] != "true" || headers[1] != "" {
		t.Fatalf("long-context headers seen: %q", headers)
	}

	li, pi := long.Identity(), plain.Identity()
	if !li.LongContext || li.WireFormat != "jev-typesafe-v1-long" || pi.LongContext || pi.WireFormat != "jev-typesafe-v1" {
		t.Fatalf("identities: long %+v plain %+v", li, pi)
	}
	if li.Endpoint != pi.Endpoint || li.Model != pi.Model {
		t.Fatalf("long context must change only the wire format: %+v vs %+v", li, pi)
	}

	_, err = New(Config{Transport: TransportGateway, APIKey: testKey, LongContext: true})
	if err == nil || !strings.Contains(err.Error(), "typesafe") {
		t.Fatalf("gateway must refuse long context, got %v", err)
	}
}
