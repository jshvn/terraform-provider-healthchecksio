package provider

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// A state from another provider has no uuid. With no uuid, the path of the request is
// the list endpoint, and the response is a blank check. Thus, Read must stop before the
// request.
func TestReadRejectsStateWithoutUUID(t *testing.T) {
	ctx := context.Background()
	hit := false
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		hit = true
		_, _ = w.Write([]byte(`{"checks": []}`))
	})
	r := &checkResource{c: c}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	state := tfsdk.State{Schema: s.Schema, Raw: checkValue(ctx, s.Schema, nil)}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if hit || !resp.Diagnostics.HasError() {
		t.Fatalf("Read with no uuid: request sent %v, diagnostics %v", hit, resp.Diagnostics)
	}
}

// The server stores the canonical name of a legacy time zone. Then the state cannot
// agree with the plan. Thus, Create deletes the new check, the error gives the canonical
// name, and the state stays empty.
func TestCreateRejectsTZChangedByServer(t *testing.T) {
	ctx := context.Background()
	const uuid = "31365bce-8da9-4729-8ff3-aaa71d56b712"
	deleted := false
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v3/checks/":
			_, _ = w.Write([]byte(`{"uuid": "` + uuid + `", "schedule": "* * * * *", "tz": "Europe/Kyiv"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v3/checks/"+uuid:
			deleted = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
	})
	r := &checkResource{c: c}
	var s resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &s)
	empty := tftypes.NewValue(tftypes.Set{ElementType: tftypes.String}, []tftypes.Value{})
	plan := tfsdk.Plan{Schema: s.Schema, Raw: checkValue(ctx, s.Schema, map[string]tftypes.Value{
		"name":       tftypes.NewValue(tftypes.String, "v"),
		"schedule":   tftypes.NewValue(tftypes.String, "* * * * *"),
		"tz":         tftypes.NewValue(tftypes.String, "Europe/Kiev"),
		"tags":       empty,
		"start_kw":   empty,
		"success_kw": empty,
		"failure_kw": empty,
	})}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: s.Schema}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	errs := resp.Diagnostics.Errors()
	if !deleted || !resp.State.Raw.IsNull() || len(errs) != 1 || !strings.Contains(errs[0].Detail(), "Use Europe/Kyiv") {
		t.Fatalf("Create: deleted %v, state %v, diagnostics %v", deleted, resp.State.Raw, resp.Diagnostics)
	}
}

// checkValue gives a check object with null attributes, except the attributes in set.
func checkValue(ctx context.Context, s schema.Schema, set map[string]tftypes.Value) tftypes.Value {
	typ := s.Type().TerraformType(ctx).(tftypes.Object)
	attrs := make(map[string]tftypes.Value, len(typ.AttributeTypes))
	for name, at := range typ.AttributeTypes {
		attrs[name] = tftypes.NewValue(at, nil)
	}
	maps.Copy(attrs, set)
	return tftypes.NewValue(typ, attrs)
}

// newTestClient gives a client for a test server with handler h. The client does not
// wait between two attempts.
func newTestClient(t *testing.T, h http.HandlerFunc) *client.Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := client.New(srv.URL+"/api/v3", strings.Repeat("k", 32), "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}
