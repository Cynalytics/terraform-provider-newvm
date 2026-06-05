package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"unithost-terraform/internal/newvm"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func encodeOrderMetaData(values []string) (string, error) {
	if len(values) == 0 {
		return "", nil
	}

	if len(values) == 1 {
		return values[0], nil
	}

	b, err := json.Marshal(values)
	if err != nil {
		return "", fmt.Errorf("failed to marshal metadata values: %w", err)
	}

	return string(b), nil
}

func decodeOrderMetaData(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}

	// If it looks like a JSON array, decode it as such.
	if len(raw) > 0 && raw[0] == '[' {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata JSON array: %w", err)
		}
		return values, nil
	}

	// Otherwise treat it as a single string value.
	return []string{raw}, nil
}

func expandOrderMetadata(ctx context.Context, metadata types.Map, orderID int) ([]newvm.NewVmOrderMetaData, diag.Diagnostics) {
	var diags diag.Diagnostics

	if metadata.IsNull() || metadata.IsUnknown() {
		return nil, diags
	}

	elements := metadata.Elements()
	result := make([]newvm.NewVmOrderMetaData, 0, len(elements))

	for key, value := range elements {
		listValue, ok := value.(types.List)
		if !ok {
			diags.AddError(
				"Invalid metadata value type",
				fmt.Sprintf("Metadata key %q is not a list of strings.", key),
			)
			continue
		}

		if listValue.IsNull() || listValue.IsUnknown() {
			continue
		}

		var values []string
		d := listValue.ElementsAs(ctx, &values, false)
		diags.Append(d...)
		if diags.HasError() {
			continue
		}

		encoded, err := encodeOrderMetaData(values)
		if err != nil {
			diags.AddError(
				"Failed to encode metadata",
				fmt.Sprintf("Could not encode metadata key %q: %s", key, err),
			)
			continue
		}

		result = append(result, newvm.NewVmOrderMetaData{
			OrderID:  orderID,
			DataType: key,
			Data:     encoded,
		})
	}

	return result, diags
}
func flattenOrderMetadata(ctx context.Context, items []newvm.NewVmOrderMetaData) (types.Map, diag.Diagnostics) {
	var diags diag.Diagnostics

	if len(items) == 0 {
		emptyMap, d := types.MapValue(
			types.ListType{ElemType: types.StringType},
			map[string]attr.Value{},
		)
		diags.Append(d...)
		return emptyMap, diags
	}

	elements := make(map[string]attr.Value, len(items))

	for _, item := range items {
		values, err := decodeOrderMetaData(item.Data)
		if err != nil {
			diags.AddError(
				"Failed to decode metadata",
				fmt.Sprintf("Could not decode metadata key %q: %s", item.DataType, err),
			)
			continue
		}

		listValues := make([]attr.Value, 0, len(values))
		for _, v := range values {
			listValues = append(listValues, types.StringValue(v))
		}

		listValue, d := types.ListValue(types.StringType, listValues)
		diags.Append(d...)
		if diags.HasError() {
			continue
		}

		elements[item.DataType] = listValue
	}

	mapValue, d := types.MapValue(
		types.ListType{ElemType: types.StringType},
		elements,
	)
	diags.Append(d...)

	return mapValue, diags
}
