package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/list"
	"github.com/hashicorp/terraform-plugin-framework/list/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

var _ list.ListResourceWithConfigure = (*checkList)(nil)

func newCheckList() list.ListResource { return &checkList{} }

// checkList finds the checks of a project for `terraform query`. With
// -generate-config-out, Terraform writes an import block and a resource block for each
// check. Thus, it is the fast way to adopt the checks that exist.
type checkList struct {
	c *client.Client
}

type checkListConfig struct {
	Slug types.String `tfsdk:"slug"`
	Tags types.List   `tfsdk:"tags"`
}

func (l *checkList) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_check"
}

func (l *checkList) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	l.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (l *checkList) ListResourceConfigSchema(_ context.Context, _ list.ListResourceSchemaRequest, resp *list.ListResourceSchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Finds the checks of the project. Without a filter, it finds all checks.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{MarkdownDescription: "Find only the checks with this slug.", Optional: true},
			"tags": schema.ListAttribute{MarkdownDescription: "Find only the checks that have all of these tags.", ElementType: types.StringType, Optional: true},
		},
	}
}

func (l *checkList) List(ctx context.Context, req list.ListRequest, stream *list.ListResultsStream) {
	var cfg checkListConfig
	diags := req.Config.Get(ctx, &cfg)
	var tags []string
	diags.Append(cfg.Tags.ElementsAs(ctx, &tags, true)...)
	if diags.HasError() {
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}
	checks, err := l.c.ListChecks(ctx, client.CheckFilter{Slug: cfg.Slug.ValueString(), Tags: tags})
	if err != nil {
		diags.AddError("Cannot list the checks", err.Error())
		stream.Results = list.ListResultsStreamDiagnostics(diags)
		return
	}
	stream.Results = func(push func(list.ListResult) bool) {
		for i := range checks {
			result := req.NewListResult(ctx)
			fields := fieldsFromAPI(&checks[i])
			result.DisplayName = checks[i].Name
			result.Diagnostics.Append(result.Identity.Set(ctx, checkIdentity{UniqueKey: fields.ID})...)
			if req.IncludeResource {
				result.Diagnostics.Append(result.Resource.Set(ctx, &fields)...)
			}
			if !push(result) {
				return
			}
		}
	}
}
