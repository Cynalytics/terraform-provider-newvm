package provider

import (
	"context"
	"fmt"
	"net"
	"strings"
	"unithost-terraform/internal/newvm"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &dnsRecordResource{}
	_ resource.ResourceWithConfigure   = &dnsRecordResource{}
	_ resource.ResourceWithImportState = &dnsRecordResource{}
)

func NewDnsRecordResource() resource.Resource {
	return &dnsRecordResource{}
}

type dnsRecordResource struct {
	client *newvm.Client
}

type dnsRecordResourceModel struct {
	Zone     types.String `tfsdk:"zone"`
	Hash     types.String `tfsdk:"hash"`
	Type     types.String `tfsdk:"type"`
	Name     types.String `tfsdk:"name"`
	TTL      types.Int64  `tfsdk:"ttl"`
	Value    types.String `tfsdk:"value"`
	Flag     types.Int64  `tfsdk:"flag"`
	Tag      types.String `tfsdk:"tag"`
	Priority types.Int64  `tfsdk:"priority"`
	Weight   types.Int64  `tfsdk:"weight"`
	Port     types.Int64  `tfsdk:"port"`
	Target   types.String `tfsdk:"target"`
}

func (r *dnsRecordResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_record"
}

func (r *dnsRecordResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*newvm.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *newvm.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	r.client = client
}

func (r *dnsRecordResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manage DNS records in a NewVM DNS zone.",
		Attributes: map[string]schema.Attribute{
			"zone": schema.StringAttribute{
				Required:    true,
				Description: "Zone name, for example domain.tld.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"hash": schema.StringAttribute{
				Computed:    true,
				Description: "NewVM DNS record hash identifier.",
			},
			"type": schema.StringAttribute{
				Required:    true,
				Description: "DNS record type. Supported: A, AAAA, CAA, CNAME, TXT.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: "Record name relative to the zone. Leave empty for the zone apex.",
			},
			"ttl": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(3600),
				Description: "TTL in seconds.",
			},
			"value": schema.StringAttribute{
				Required:    true,
				Description: "Record content/value.",
			},
			"flag": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Default:     int64default.StaticInt64(0),
				Description: "CAA flag.",
			},
			"tag": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(""),
				Description: "CAA tag, for example issue, issuewild or iodef.",
			},
			"priority": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Reserved for future MX/SRV support.",
			},
			"weight": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Reserved for future SRV support.",
			},
			"port": schema.Int64Attribute{
				Optional:    true,
				Computed:    true,
				Description: "Reserved for future SRV support.",
			},
			"target": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Reserved for future MX/SRV support.",
			},
		},
	}
}

func (r *dnsRecordResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsRecordResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.validateModel(&plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	beforeZone, err := r.client.GetZone(ctx, plan.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read DNS zone",
			err.Error(),
		)
		return
	}

	existingHashes := make(map[string]struct{}, len(beforeZone.Zone.Records))
	for _, record := range beforeZone.Zone.Records {
		existingHashes[record.Hash] = struct{}{}
	}

	recordType := strings.ToUpper(plan.Type.ValueString())
	normalizedValue := newvm.NormalizeDnsRecordValue(recordType, plan.Value.ValueString())
	normalizedTarget := newvm.NormalizeDnsRecordValue(recordType, plan.Target.ValueString())

	createReq := newvm.DnsRecordCreateRequest{
		ClientID: beforeZone.Zone.OwnerID,
		Type:     recordType,
		Name:     plan.Name.ValueString(),
		TTL:      plan.TTL.ValueInt64(),
		Value:    normalizedValue,
		Tag:      plan.Tag.ValueString(),
		Target:   normalizedTarget,
	}

	if !plan.Flag.IsNull() && !plan.Flag.IsUnknown() {
		flag := plan.Flag.ValueInt64()
		createReq.Flag = &flag
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		priority := plan.Priority.ValueInt64()
		createReq.Priority = &priority
	}
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		weight := plan.Weight.ValueInt64()
		createReq.Weight = &weight
	}
	if !plan.Port.IsNull() && !plan.Port.IsUnknown() {
		port := plan.Port.ValueInt64()
		createReq.Port = &port
	}

	if _, err := r.client.CreateDnsRecord(ctx, plan.Zone.ValueString(), createReq); err != nil {
		resp.Diagnostics.AddError(
			"Unable to create DNS record",
			err.Error(),
		)
		return
	}

	readBack, err := r.client.GetZone(ctx, plan.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"DNS record created but could not re-read zone",
			err.Error(),
		)
		return
	}

	want := newvm.DnsRecord{
		Type:     recordType,
		Name:     plan.Name.ValueString(),
		TTL:      plan.TTL.ValueInt64(),
		Content:  normalizedValue,
		Value:    normalizedValue,
		Flag:     int64Value(plan.Flag),
		Tag:      plan.Tag.ValueString(),
		Priority: int64Value(plan.Priority),
		Weight:   int64Value(plan.Weight),
		Port:     int64Value(plan.Port),
		Target:   normalizedTarget,
	}

	var record *newvm.DnsRecord
	for i := range readBack.Zone.Records {
		rb := &readBack.Zone.Records[i]

		if _, existed := existingHashes[rb.Hash]; existed {
			continue
		}

		if newvm.DnsRecordMatches(*rb, want) {
			record = rb
			break
		}
	}

	if record == nil {
		record = newvm.FindDnsRecord(readBack.Zone.Records, want)
	}

	if record == nil {
		resp.Diagnostics.AddError(
			"Unable to locate created DNS record",
			"The create call succeeded, but the created record could not be matched in the zone listing.",
		)
		return
	}

	r.apiRecordToState(plan.Zone.ValueString(), record, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsRecordResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	zone, err := r.client.GetZone(ctx, state.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to read DNS zone",
			err.Error(),
		)
		return
	}

	record := newvm.FindDnsRecordByHash(zone.Zone.Records, state.Hash.ValueString())
	if record == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	r.apiRecordToState(state.Zone.ValueString(), record, &state)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsRecordResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsRecordResourceModel
	var state dnsRecordResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.validateModel(&plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	recordType := strings.ToUpper(plan.Type.ValueString())
	normalizedValue := newvm.NormalizeDnsRecordValue(recordType, plan.Value.ValueString())
	normalizedTarget := newvm.NormalizeDnsRecordValue(recordType, plan.Target.ValueString())

	updateReq := newvm.DnsRecordUpdateRequest{
		Hash:   state.Hash.ValueString(),
		Name:   plan.Name.ValueString(),
		TTL:    plan.TTL.ValueInt64(),
		Value:  normalizedValue,
		Tag:    plan.Tag.ValueString(),
		Target: normalizedTarget,
	}

	if !plan.Flag.IsNull() && !plan.Flag.IsUnknown() {
		flag := plan.Flag.ValueInt64()
		updateReq.Flag = &flag
	}
	if !plan.Priority.IsNull() && !plan.Priority.IsUnknown() {
		priority := plan.Priority.ValueInt64()
		updateReq.Priority = &priority
	}
	if !plan.Weight.IsNull() && !plan.Weight.IsUnknown() {
		weight := plan.Weight.ValueInt64()
		updateReq.Weight = &weight
	}
	if !plan.Port.IsNull() && !plan.Port.IsUnknown() {
		port := plan.Port.ValueInt64()
		updateReq.Port = &port
	}

	if _, err := r.client.UpdateDnsRecord(ctx, plan.Zone.ValueString(), updateReq); err != nil {
		resp.Diagnostics.AddError(
			"Unable to update DNS record",
			err.Error(),
		)
		return
	}

	readBack, err := r.client.GetZone(ctx, plan.Zone.ValueString())
	if err != nil {
		resp.Diagnostics.AddError(
			"DNS record updated but could not re-read zone",
			err.Error(),
		)
		return
	}

	record := newvm.FindDnsRecordByHash(readBack.Zone.Records, state.Hash.ValueString())
	if record == nil {
		want := newvm.DnsRecord{
			Type:     recordType,
			Name:     plan.Name.ValueString(),
			TTL:      plan.TTL.ValueInt64(),
			Content:  normalizedValue,
			Value:    normalizedValue,
			Flag:     int64Value(plan.Flag),
			Tag:      plan.Tag.ValueString(),
			Priority: int64Value(plan.Priority),
			Weight:   int64Value(plan.Weight),
			Port:     int64Value(plan.Port),
			Target:   normalizedTarget,
		}

		record = newvm.FindDnsRecord(readBack.Zone.Records, want)
	}

	if record == nil {
		resp.Diagnostics.AddError(
			"Unable to locate updated DNS record",
			"The update call succeeded, but the record could not be found afterwards.",
		)
		return
	}

	r.apiRecordToState(plan.Zone.ValueString(), record, &plan)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *dnsRecordResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state dnsRecordResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := r.client.DeleteDnsRecord(ctx, state.Zone.ValueString(), state.Hash.ValueString()); err != nil {
		resp.Diagnostics.AddError(
			"Unable to delete DNS record",
			err.Error(),
		)
		return
	}
}

func (r *dnsRecordResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		resp.Diagnostics.AddError(
			"Unexpected import ID",
			`Expected import ID in the format "zone/hash", for example "domain.tld/1facf0d3ad979e38309c254521d1589fa8637059".`,
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("zone"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("hash"), parts[1])...)
}

func (r *dnsRecordResource) validateModel(model *dnsRecordResourceModel, diags *diag.Diagnostics) {
	recordType := strings.ToUpper(strings.TrimSpace(model.Type.ValueString()))

	switch recordType {
	case "A", "AAAA", "CAA", "CNAME", "TXT":
		// ok
	default:
		diags.AddAttributeError(
			path.Root("type"),
			"Unsupported DNS record type",
			`Supported types are: A, AAAA, CAA, CNAME and TXT.`,
		)
	}

	if model.TTL.ValueInt64() <= 0 {
		diags.AddAttributeError(
			path.Root("ttl"),
			"Invalid TTL",
			"TTL must be greater than zero.",
		)
	}

	if strings.TrimSpace(model.Value.ValueString()) == "" {
		diags.AddAttributeError(
			path.Root("value"),
			"Missing record value",
			"The value attribute must not be empty.",
		)
	}

	if recordType == "A" {
		ip := net.ParseIP(strings.TrimSpace(model.Value.ValueString()))
		if ip == nil || ip.To4() == nil {
			diags.AddAttributeError(
				path.Root("value"),
				"Invalid IPv4 address",
				`For type "A", value must contain a valid IPv4 address.`,
			)
		}
	}

	if recordType == "AAAA" {
		ip := net.ParseIP(strings.TrimSpace(model.Value.ValueString()))
		if ip == nil || ip.To4() != nil {
			diags.AddAttributeError(
				path.Root("value"),
				"Invalid IPv6 address",
				`For type "AAAA", value must contain a valid IPv6 address.`,
			)
		}
	}

	if recordType == "CAA" && strings.TrimSpace(model.Tag.ValueString()) == "" {
		diags.AddAttributeError(
			path.Root("tag"),
			"Missing CAA tag",
			`For CAA records, "tag" is required.`,
		)
	}
}

func (r *dnsRecordResource) apiRecordToState(zone string, record *newvm.DnsRecord, state *dnsRecordResourceModel) {
	state.Zone = types.StringValue(zone)
	state.Hash = types.StringValue(record.Hash)
	state.Type = types.StringValue(strings.ToUpper(record.Type))
	state.Name = types.StringValue(record.Name)
	state.TTL = types.Int64Value(record.TTL)

	switch strings.ToUpper(strings.TrimSpace(record.Type)) {
	case "CAA":
		value := record.Value
		if value == "" {
			value = record.Content
		}

		state.Value = types.StringValue(newvm.NormalizeDnsRecordValue(record.Type, value))
		state.Flag = types.Int64Value(record.Flag)
		state.Tag = types.StringValue(record.Tag)

	case "CNAME":
		value := record.Target
		if value == "" {
			value = record.Content
		}

		state.Value = types.StringValue(newvm.NormalizeDnsRecordValue(record.Type, value))
		state.Flag = types.Int64Value(0)
		state.Tag = types.StringValue("")

	default:
		state.Value = types.StringValue(newvm.NormalizeDnsRecordValue(record.Type, record.Content))
		state.Flag = types.Int64Value(0)
		state.Tag = types.StringValue("")
	}

	state.Priority = types.Int64Value(record.Priority)
	state.Weight = types.Int64Value(record.Weight)
	state.Port = types.Int64Value(record.Port)
	state.Target = types.StringValue(newvm.NormalizeDnsRecordValue(record.Type, record.Target))
}

func int64Value(v types.Int64) int64 {
	if v.IsNull() || v.IsUnknown() {
		return 0
	}

	return v.ValueInt64()
}
