package newvm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

type newVmOrderMetaShortResponse map[string]interface{}

func (c *Client) GetOrderMetaData(ctx context.Context, orderID int64) ([]NewVmOrderMetaData, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/account/v1/customer/self/order/%s/meta", c.HostURL, strconv.FormatInt(orderID, 10)),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var items []NewVmOrderMetaData
	if err := json.Unmarshal(body, &items); err == nil {
		return items, nil
	}

	type wrapped struct {
		Result []NewVmOrderMetaData `json:"result"`
		Meta   []NewVmOrderMetaData `json:"meta"`
		Items  []NewVmOrderMetaData `json:"items"`
		Data   []NewVmOrderMetaData `json:"data"`
	}
	var w wrapped
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, err
	}

	switch {
	case len(w.Result) > 0:
		return w.Result, nil
	case len(w.Meta) > 0:
		return w.Meta, nil
	case len(w.Items) > 0:
		return w.Items, nil
	case len(w.Data) > 0:
		return w.Data, nil
	default:
		return []NewVmOrderMetaData{}, nil
	}
}

func (c *Client) GetOrderMetaDataShort(ctx context.Context, orderID int64) (map[string][]string, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		fmt.Sprintf("%s/account/v1/customer/self/order/%s/meta/short", c.HostURL, strconv.FormatInt(orderID, 10)),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	raw := map[string]interface{}{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	result := make(map[string][]string, len(raw))
	for key, value := range raw {
		switch v := value.(type) {
		case string:
			result[key] = []string{v}
		case []interface{}:
			values := make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					values = append(values, s)
				}
			}
			result[key] = values
		}
	}

	return result, nil
}

func (c *Client) CreateOrderMetaData(ctx context.Context, orderID int64, item NewVmOrderMetaData) (*NewVmOrderMetaData, error) {
	item.OrderID = int(orderID)

	rb, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/account/v1/customer/self/order/%s/meta", c.HostURL, strconv.FormatInt(orderID, 10)),
		strings.NewReader(string(rb)),
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var created NewVmOrderMetaData
	if err := json.Unmarshal(body, &created); err != nil {
		return &item, nil
	}

	return &created, nil
}

func (c *Client) UpdateOrderMetaData(ctx context.Context, orderID int64, item NewVmOrderMetaData) (*NewVmOrderMetaData, error) {
	item.OrderID = int(orderID)

	rb, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPatch,
		fmt.Sprintf("%s/account/v1/customer/self/order/%s/meta", c.HostURL, strconv.FormatInt(orderID, 10)),
		strings.NewReader(string(rb)),
	)
	if err != nil {
		return nil, err
	}

	body, err := c.doRequest(req)
	if err != nil {
		return nil, err
	}

	var updated NewVmOrderMetaData
	if err := json.Unmarshal(body, &updated); err != nil {
		return &item, nil
	}

	return &updated, nil
}

func (c *Client) DeleteOrderMetaData(ctx context.Context, orderID int64, orderMetaID string, dataType string) error {
	type deleteOrderMetaRequest struct {
		ID       string `json:"id,omitempty"`
		DataType string `json:"dataType,omitempty"`
	}

	payload := deleteOrderMetaRequest{
		ID:       orderMetaID,
		DataType: dataType,
	}

	rb, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("%s/account/v1/customer/self/order/%s/meta", c.HostURL, strconv.FormatInt(orderID, 10)),
		strings.NewReader(string(rb)),
	)
	if err != nil {
		return err
	}

	_, err = c.doRequest(req)
	return err
}

func (c *Client) SyncOrderMetaData(ctx context.Context, orderID int64, desired []NewVmOrderMetaData) error {
	current, err := c.GetOrderMetaData(ctx, orderID)
	if err != nil {
		return err
	}

	currentByType := make(map[string]NewVmOrderMetaData, len(current))
	for _, item := range current {
		currentByType[item.DataType] = item
	}

	desiredByType := make(map[string]NewVmOrderMetaData, len(desired))
	for _, item := range desired {
		item.OrderID = int(orderID)
		desiredByType[item.DataType] = item
	}

	for dataType, currentItem := range currentByType {
		if _, ok := desiredByType[dataType]; !ok {
			if err := c.DeleteOrderMetaData(ctx, orderID, currentItem.ID, currentItem.DataType); err != nil {
				return fmt.Errorf("failed to delete metadata %q: %w", dataType, err)
			}
		}
	}

	for dataType, desiredItem := range desiredByType {
		if currentItem, ok := currentByType[dataType]; ok {
			if currentItem.Data == desiredItem.Data {
				continue
			}

			desiredItem.ID = currentItem.ID
			if desiredItem.Changeable == nil {
				desiredItem.Changeable = currentItem.Changeable
			}

			if _, err := c.UpdateOrderMetaData(ctx, orderID, desiredItem); err != nil {
				return fmt.Errorf("failed to update metadata %q: %w", dataType, err)
			}
			continue
		}

		if _, err := c.CreateOrderMetaData(ctx, orderID, desiredItem); err != nil {
			return fmt.Errorf("failed to create metadata %q: %w", dataType, err)
		}
	}

	return nil
}
