package apiclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"reflect"

	"github.com/tryhisar/hisar/core/apitypes"
)

type collectionResponse struct {
	page     apitypes.CollectionPage
	target   any
	isLegacy bool
}

func (response *collectionResponse) decodeResponse(header http.Header, data []byte) error {
	if version := header.Get(apitypes.CollectionPageHeader); version != "" {
		if version != apitypes.CollectionPageVersion || len(data) > apitypes.CollectionPageBytes {
			return errors.New("server: invalid collection protocol or page byte budget")
		}
		if err := json.Unmarshal(data, &response.page); err != nil {
			return err
		}
		if response.page.Version != version || response.page.Records == nil {
			return errors.New("server: invalid collection page version or records")
		}
		return nil
	}
	return response.UnmarshalJSON(data)
}
func (response *collectionResponse) UnmarshalJSON(data []byte) error {
	var shape map[string]json.RawMessage
	if len(data) > 0 && data[0] == '{' {
		if err := json.Unmarshal(data, &shape); err != nil {
			return err
		}
		_, version := shape["version"]
		_, records := shape["records"]
		_, cursor := shape["nextCursor"]
		if version || records || cursor {
			if len(data) > apitypes.CollectionPageBytes {
				return errors.New("server: collection page exceeds the byte budget")
			}
			if err := json.Unmarshal(data, &response.page); err != nil {
				return err
			}
			if response.page.Version != apitypes.CollectionPageVersion || response.page.Records == nil {
				return errors.New("server: invalid collection page version or records")
			}
			return nil
		}
	}
	response.isLegacy = true
	return json.Unmarshal(data, response.target)
}

// collectionRequest installs no partial result. Mutation invalidates the entire traversal,
// including nested edges, instead of presenting a mixed snapshot as complete.
func (c *Client) collectionRequest(ctx context.Context, path string, target any) error {
	for range 3 {
		result := reflect.New(reflect.TypeOf(target).Elem()).Interface()
		value := reflect.ValueOf(result).Elem()
		if value.Kind() == reflect.Slice {
			value.Set(reflect.MakeSlice(value.Type(), 0, 0))
		}
		if value.Kind() == reflect.Struct {
			for index := range value.NumField() {
				field := value.Field(index)
				if field.CanSet() && field.Kind() == reflect.Slice {
					field.Set(reflect.MakeSlice(field.Type(), 0, 0))
				}
			}
		}
		cursor, lastKey := "", ""
		assembler := apitypes.NewCollectionAssembler(result)
		snapshot, planVersion := "", ""
		restart := false
		for {
			response := collectionResponse{target: result}
			err := c.sendRequest(ctx, "GET", path+"?cursor="+url.QueryEscape(cursor), http.Header{apitypes.CollectionPageHeader: {apitypes.CollectionPageVersion}}, nil, &response, nil)
			if err != nil {
				var apiErr *APIError
				if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict && apiErr.Message == apitypes.CollectionChangedMessage {
					restart = true
					break
				}
				return err
			}
			if response.isLegacy {
				if cursor != "" {
					return errors.New("server: collection protocol changed during traversal")
				}
				break
			}
			page := response.page
			if page.Version != apitypes.CollectionPageVersion || page.Records == nil || len(page.Records) > apitypes.CollectionPageRows || len(page.NextCursor) > 4096 {
				return errors.New("server: invalid collection page")
			}
			if lastKey == "" {
				snapshot, planVersion = page.Snapshot, page.PlanVersion
			} else if snapshot != page.Snapshot || planVersion != page.PlanVersion {
				return errors.New("server: collection snapshot or deletion plan changed between pages")
			}
			for _, record := range page.Records {
				if record.Key <= lastKey {
					return errors.New("server: unordered or repeated collection record")
				}
				if err := assembler.Apply(record); err != nil {
					return err
				}
				lastKey = record.Key
			}
			if page.NextCursor == "" {
				break
			}
			if len(page.Records) == 0 || page.NextCursor == cursor {
				return errors.New("server: collection cursor made no progress")
			}
			cursor = page.NextCursor
		}
		if restart {
			continue
		}
		assembler.Finish()
		reflect.ValueOf(target).Elem().Set(reflect.ValueOf(result).Elem())
		return nil
	}
	return &APIError{Status: http.StatusConflict, Message: apitypes.CollectionChangedMessage}
}
