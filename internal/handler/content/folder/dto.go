package folder

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	dbgen "github.com/Harshwagh21/NimbusCloud/db/generated"
)

type createRequest struct {
	Name     string     `json:"name"`
	ParentID *uuid.UUID `json:"parent_id"`
}

// updateRequest distinguishes omitted parent_id from explicit null (move to root).
type updateRequest struct {
	Name      *string
	ParentID  *uuid.UUID
	ParentSet bool
}

func (r *updateRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if nameRaw, ok := raw["name"]; ok {
		var name string
		if err := json.Unmarshal(nameRaw, &name); err != nil {
			return err
		}
		r.Name = &name
	}
	if parentRaw, ok := raw["parent_id"]; ok {
		r.ParentSet = true
		if string(parentRaw) == "null" {
			r.ParentID = nil
			return nil
		}
		var id uuid.UUID
		if err := json.Unmarshal(parentRaw, &id); err != nil {
			return err
		}
		r.ParentID = &id
	}
	return nil
}

type folderResponse struct {
	ID        uuid.UUID  `json:"id"`
	ParentID  *uuid.UUID `json:"parent_id"`
	Name      string     `json:"name"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type foldersResponse struct {
	Folders []folderResponse `json:"folders"`
}

type pathResponse struct {
	Path []folderResponse `json:"path"`
}

func newFolderResponse(folder dbgen.Folder) folderResponse {
	return folderResponse{
		ID:        folder.ID,
		ParentID:  folder.ParentID,
		Name:      folder.Name,
		CreatedAt: folder.CreatedAt,
		UpdatedAt: folder.UpdatedAt,
	}
}

func newFoldersResponse(folders []dbgen.Folder) foldersResponse {
	out := make([]folderResponse, 0, len(folders))
	for _, folder := range folders {
		out = append(out, newFolderResponse(folder))
	}
	return foldersResponse{Folders: out}
}

func newPathResponse(folders []dbgen.Folder) pathResponse {
	return pathResponse{Path: newFoldersResponse(folders).Folders}
}
