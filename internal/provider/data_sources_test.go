package provider

import (
	"context"
	"net/http"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The server answers 500 when it cannot query its database. Then ok is false, and the
// read does not fail.
func TestStatusIsFalseOn5xx(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	d := &statusDataSource{dataSourceBase{name: "status", c: c}}
	var s datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &s)
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: s.Schema}}
	d.Read(ctx, datasource.ReadRequest{}, &resp)
	var got statusModel
	resp.Diagnostics.Append(resp.State.Get(ctx, &got)...)
	if resp.Diagnostics.HasError() || !got.OK.Equal(types.BoolValue(false)) {
		t.Fatalf("Read after 500: diagnostics %v, ok %v", resp.Diagnostics, got.OK)
	}
}
