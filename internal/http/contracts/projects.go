package contracts

import "time"

// CreateProjectRequest описывает создание нового проекта пользователя.
type CreateProjectRequest struct {
	Title              string `json:"title"`
	Marketplace        string `json:"marketplace,omitempty"`
	ProductName        string `json:"product_name,omitempty"`
	ProductDescription string `json:"product_description,omitempty"`
}

// PatchProjectRequest описывает частичное обновление проекта.
// nil означает, что поле не передавалось и менять его не нужно.
type PatchProjectRequest struct {
	Title              *string `json:"title,omitempty"`
	Marketplace        *string `json:"marketplace,omitempty"`
	ProductName        *string `json:"product_name,omitempty"`
	ProductDescription *string `json:"product_description,omitempty"`
}

// Project описывает проект в публичном HTTP API.
type Project struct {
	ID                 string    `json:"id"`
	Title              string    `json:"title"`
	Marketplace        string    `json:"marketplace,omitempty"`
	ProductName        string    `json:"product_name,omitempty"`
	ProductDescription string    `json:"product_description,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// ProjectResponse возвращает один проект.
type ProjectResponse struct {
	Project Project `json:"project"`
}

// ProjectListResponse возвращает список проектов текущего пользователя.
type ProjectListResponse struct {
	Projects []Project `json:"projects"`
}
