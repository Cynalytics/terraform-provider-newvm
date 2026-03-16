package provider

import (
	"context"
	"fmt"
	"log"
	"strconv"

	"unithost-terraform/internal/newvm"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/float64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Ensure the implementation satisfies the expected interfaces.
var (
	_ resource.Resource                = &controlPanelResource{}
	_ resource.ResourceWithConfigure   = &controlPanelResource{}
	_ resource.ResourceWithImportState = &controlPanelResource{}
)

// NewControlPanelResource is a helper function to simplify the provider implementation.
func NewControlPanelResource() resource.Resource {
	return &controlPanelResource{}
}

// controlPanelExtensionResourceModel maps the data source schema data.
type controlPanelExtensionResourceModel struct {
	ID          types.String  `tfsdk:"id"`
	Description types.String  `tfsdk:"description"`
	Price       types.Float64 `tfsdk:"price"`
}

// controlPanelResourceModel maps the resource schema data.
type controlPanelResourceModel struct {
	ID         types.Int64                          `tfsdk:"id"`
	ProductID  types.String                         `tfsdk:"product_id"`
	VmOrderID  types.Int64                          `tfsdk:"vm_order_id"`
	Extensions []controlPanelExtensionResourceModel `tfsdk:"extensions"`
	Metadata   types.Map                            `tfsdk:"metadata"`
	// LastUpdated types.String                         `tfsdk:"last_updated"`
}

// controlPanelResource is the resource implementation.
type controlPanelResource struct {
	client *newvm.Client
}

// Metadata returns the resource type name.
func (r *controlPanelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_control_panel"
}

// Schema defines the schema for the resource.
func (r *controlPanelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Used to construct empty list default for extensions
	var extensionObjectType = types.ObjectType{
		AttrTypes: map[string]attr.Type{
			"id":          types.StringType,
			"description": types.StringType,
			"price":       types.Float64Type,
		},
	}
	resp.Schema = schema.Schema{
		Description: "Manages a Control Panel.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Description: "Numeric identifier of the control panel. (order number)",
				Computed:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.UseStateForUnknown(),
				},
			},
			"vm_order_id": schema.Int64Attribute{
				Description: "order ID of the VM where the control panel is for.",
				Required:    true,
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"product_id": schema.StringAttribute{
				Description: "product ID of the control panel. (eg. 'CP_PLESK.plesk_12_license.1')",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"extensions": schema.SetNestedAttribute{
				Computed: true,
				Optional: true,
				Default:  setdefault.StaticValue(types.SetValueMust(extensionObjectType, []attr.Value{})),
				PlanModifiers: []planmodifier.Set{
					setplanmodifier.UseStateForUnknown(),
				},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id": schema.StringAttribute{
							Description: "the ID of the control panel extension.",
							Required:    true,
						},
						"description": schema.StringAttribute{
							Description: "description of the control panel extension.",
							Computed:    true,
							PlanModifiers: []planmodifier.String{
								stringplanmodifier.UseStateForUnknown(),
							},
						},
						"price": schema.Float64Attribute{
							Description: "price of the control panel extension.",
							Computed:    true,
							PlanModifiers: []planmodifier.Float64{
								float64planmodifier.UseStateForUnknown(),
							},
						},
					},
				},
			},
			"metadata": schema.MapAttribute{
				Optional:    true,
				Description: "Order metadata as a map of key => list of values.",
				ElementType: types.ListType{
					ElemType: types.StringType,
				},
			},
			// "last_updated": schema.StringAttribute{
			// Description: "Timestamp of the last Terraform update of the control panel.",
			// Computed:    true,
			// PlanModifiers: []planmodifier.String{
			// stringplanmodifier.UseStateForUnknown(),
			// },
			// },
		},
	}
}

// Create a new control panel resource.
func (r *controlPanelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	// Retrieve values from plan
	var plan controlPanelResourceModel
	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	newControlPanelOrder := newvm.ControlPanel{
		ID:         int(plan.ID.ValueInt64()),
		ProductID:  plan.ProductID.ValueString(),
		VmOrderID:  int(plan.VmOrderID.ValueInt64()),
		Extensions: []newvm.ControlPanelExtension{},
	}
	for _, extension := range plan.Extensions {
		newControlPanelOrder.Extensions = append(newControlPanelOrder.Extensions, newvm.ControlPanelExtension{
			ID:          extension.ID.ValueString(),
			Description: extension.Description.ValueString(),
			Price:       extension.Price.ValueFloat64(),
		})
	}

	// Create new control panel
	controlPanel, err := r.client.CreateControlPanel(ctx, newControlPanelOrder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error creating control panel",
			"Could not create control panel, unexpected error: "+err.Error(),
		)
		return
	}

	// Map response body to schema and populate Computed attribute values
	plan.ID = types.Int64Value(int64(controlPanel.ID))
	// plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	metadataItems, d := expandOrderMetadata(ctx, plan.Metadata, int(controlPanel.ID))
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.SyncOrderMetaData(ctx, int64(controlPanel.ID), metadataItems); err != nil {
		resp.Diagnostics.AddError("Error syncing control panel metadata", "Control panel was created, but metadata could not be synced: "+err.Error())
		return
	}

	cp, err := r.client.GetControlPanel(ctx, int64(controlPanel.ID))
	if err == nil {
		plan.Extensions = mergeExtensionsByID(plan.Extensions, cp.Extensions)
		plan.VmOrderID = types.Int64Value(int64(cp.VmOrderID))
		plan.ProductID = types.StringValue(cp.ProductID)
	} else {
		// normalize unknowns so no unknowns remain after apply
		plan.Extensions = mergeExtensionsByID(plan.Extensions, nil)
	}

	metaItems, err := r.client.GetOrderMetaData(ctx, int64(controlPanel.ID))
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading control panel metadata",
			"Control panel was created, but metadata could not be read back: "+err.Error(),
		)
		return
	}

	plan.Metadata, d = flattenOrderMetadata(ctx, metaItems)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

// Read resource information.
func (r *controlPanelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	// Get current state
	var state controlPanelResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	controlPanelID := state.ID.ValueInt64()
	if controlPanelID <= 0 {
		return
	}

	log.Println("Reading control panel: ", controlPanelID)

	controlPanel, err := r.client.GetControlPanel(ctx, controlPanelID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading control panel",
			"Could not read control panel "+strconv.FormatInt(controlPanelID, 10)+": "+err.Error(),
		)
		return
	}

	// Overwrite items with refreshed state
	state.VmOrderID = types.Int64Value(int64(controlPanel.VmOrderID))
	state.ProductID = types.StringValue(controlPanel.ProductID)
	// Merge API into current state; if API omits an extension briefly,
	// the union keeps it instead of dropping it and causing thrash.
	state.Extensions = mergeExtensionsByID(state.Extensions, controlPanel.Extensions)

	metaItems, err := r.client.GetOrderMetaData(ctx, controlPanelID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Reading control panel metadata",
			"Could not read metadata for control panel "+strconv.FormatInt(controlPanelID, 10)+": "+err.Error(),
		)
		return
	}

	state.Metadata, diags = flattenOrderMetadata(ctx, metaItems)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		log.Printf("Error updating state: %v", resp.Diagnostics.Errors())
		return
	}

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		log.Printf("Error updating state: %v", resp.Diagnostics.Errors())
		return
	}
}

func (r *controlPanelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// Retrieve values from plan
	var plan controlPanelResourceModel

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Generate API request body from plan
	newControlPanelOrder := newvm.ControlPanel{
		ProductID:  plan.ProductID.ValueString(),
		VmOrderID:  int(plan.VmOrderID.ValueInt64()),
		Extensions: []newvm.ControlPanelExtension{},
	}
	for _, extension := range plan.Extensions {
		newControlPanelOrder.Extensions = append(newControlPanelOrder.Extensions, newvm.ControlPanelExtension{
			ID:          extension.ID.ValueString(),
			Description: extension.Description.ValueString(),
			Price:       extension.Price.ValueFloat64(),
		})
	}

	// Update existing control panel
	_, err := r.client.UpdateControlPanel(ctx, plan.ID.ValueInt64(), newControlPanelOrder)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Updating NewVM control panel",
			"Could not update control panel, unexpected error: "+err.Error(),
		)
		return
	}

	metadataItems, d := expandOrderMetadata(ctx, plan.Metadata, int(plan.ID.ValueInt64()))
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.SyncOrderMetaData(ctx, plan.ID.ValueInt64(), metadataItems); err != nil {
		resp.Diagnostics.AddError(
			"Error syncing control panel metadata",
			"Could not sync metadata for control panel: "+err.Error(),
		)
		return
	}

	cp, err := r.client.GetControlPanel(ctx, plan.ID.ValueInt64())
	if err != nil {
		// fallback: keep plan (normalized) so elements don't "vanish"
		plan.Extensions = mergeExtensionsByID(plan.Extensions, nil)
	} else {
		plan.Extensions = mergeExtensionsByID(plan.Extensions, cp.Extensions)
		plan.VmOrderID = types.Int64Value(int64(cp.VmOrderID))
		plan.ProductID = types.StringValue(cp.ProductID)
	}
	// plan.LastUpdated = types.StringValue(time.Now().Format(time.RFC850))

	metaItems, err := r.client.GetOrderMetaData(ctx, plan.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError(
			"Error reading control panel metadata",
			"Could not read metadata after update: "+err.Error(),
		)
		return
	}

	plan.Metadata, d = flattenOrderMetadata(ctx, metaItems)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}

	diags = resp.State.Set(ctx, plan)
	resp.Diagnostics.Append(diags...)
}

func (r *controlPanelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Retrieve values from state
	var state controlPanelResourceModel
	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	controlPanelID := state.ID.ValueInt64()
	if controlPanelID <= 0 {
		resp.Diagnostics.AddError(
			"Error Deleting control panel",
			"Could not delete control panel, no ID given",
		)
		return
	}

	err := r.client.DeleteControlPanel(ctx, controlPanelID)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error Deleting control panel",
			"Could not delete control panel, unexpected error: "+err.Error(),
		)
		return
	}
}

// Configure adds the provider configured client to the resource.
func (r *controlPanelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	// Add a nil check when handling ProviderData because Terraform
	// sets that data after it calls the ConfigureProvider RPC.
	if req.ProviderData == nil {
		return
	}

	client, ok := req.ProviderData.(*newvm.Client)

	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *newvm.Client, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)

		return
	}

	r.client = client
}

func (r *controlPanelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected numeric control panel ID, got %q: %s", req.ID, err),
		)
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// mergeExtensionsByID returns a stable union of plan and api by extension ID.
// - If API has a given ID, prefer its values (known desc/price).
// - Otherwise keep the planned element (so it doesn't "vanish").
// - Ensures no Unknown values remain (use Null for unknowns).
func mergeExtensionsByID(
	plan []controlPanelExtensionResourceModel,
	api []newvm.ControlPanelExtension,
) []controlPanelExtensionResourceModel {
	byPlan := map[string]controlPanelExtensionResourceModel{}
	for _, p := range plan {
		// normalize unknowns to Null so state is valid after apply
		if p.Description.IsUnknown() {
			p.Description = types.StringNull()
		}
		if p.Price.IsUnknown() {
			p.Price = types.Float64Null()
		}
		byPlan[p.ID.ValueString()] = p
	}

	byAPI := map[string]newvm.ControlPanelExtension{}
	for _, a := range api {
		byAPI[a.ID] = a
	}

	// Union of IDs, prefer API payload if present
	out := make([]controlPanelExtensionResourceModel, 0, len(byPlan)+len(byAPI))
	seen := map[string]struct{}{}

	for id, a := range byAPI {
		out = append(out, controlPanelExtensionResourceModel{
			ID:          types.StringValue(id),
			Description: types.StringValue(a.Description),
			Price:       types.Float64Value(a.Price),
		})
		seen[id] = struct{}{}
	}
	for id, p := range byPlan {
		if _, ok := seen[id]; ok {
			continue
		}
		// keep planned element (desc/price already normalized to Null above)
		out = append(out, controlPanelExtensionResourceModel{
			ID:          types.StringValue(id),
			Description: p.Description,
			Price:       p.Price,
		})
	}

	return out
}
