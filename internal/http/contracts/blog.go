// Package contracts содержит transport DTO для публичных HTTP-контрактов.
package contracts

import "time"

// BlogListResponse описывает ответ для страницы списка статей `/blog`.
type BlogListResponse struct {
	FeaturedPost *FeaturedBlogPost `json:"featured_post,omitempty"`
	Posts        []BlogListItem    `json:"posts"`
	Categories   []BlogCategory    `json:"categories"`
	PopularPosts []BlogSidebarPost `json:"popular_posts"`
	Tags         []BlogTag         `json:"tags"`
	Pagination   Pagination        `json:"pagination"`
}

// FeaturedBlogPost описывает главную статью вверху списка блога.
type FeaturedBlogPost struct {
	Slug               string    `json:"slug"`
	Title              string    `json:"title"`
	Excerpt            string    `json:"excerpt"`
	PublishedAt        time.Time `json:"published_at"`
	ReadingTimeMinutes int       `json:"reading_time_minutes"`
	CanonicalPath      string    `json:"canonical_path"`
}

// BlogListItem описывает одну карточку статьи в списке блога.
type BlogListItem struct {
	Slug               string    `json:"slug"`
	Title              string    `json:"title"`
	Excerpt            string    `json:"excerpt"`
	CategoryLabel      string    `json:"category_label"`
	PublishedAt        time.Time `json:"published_at"`
	ReadingTimeMinutes int       `json:"reading_time_minutes"`
	CanonicalPath      string    `json:"canonical_path"`
}

// BlogCategory описывает категорию статьи и количество материалов в ней.
type BlogCategory struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// BlogSidebarPost описывает короткую ссылку на статью в боковом блоке.
type BlogSidebarPost struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	CanonicalPath string `json:"canonical_path"`
}

// BlogTag описывает тег статьи в публичном списке блога.
type BlogTag struct {
	Slug  string `json:"slug"`
	Label string `json:"label"`
}

// Pagination описывает серверную пагинацию для SEO-страниц списка.
type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}

// BlogPostResponse описывает ответ для страницы статьи `/blog/{slug}`.
type BlogPostResponse struct {
	Post            BlogPostDetail       `json:"post"`
	ArticleSections []BlogArticleSection `json:"article_sections"`
	RelatedPosts    []RelatedBlogPost    `json:"related_posts"`
}

// BlogPostDetail содержит основные данные статьи для шапки и SEO.
type BlogPostDetail struct {
	Slug               string    `json:"slug"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Excerpt            string    `json:"excerpt"`
	CanonicalPath      string    `json:"canonical_path"`
	PublishedAt        time.Time `json:"published_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	ReadingTimeMinutes int       `json:"reading_time_minutes"`
	Views              int       `json:"views"`
	Tags               []BlogTag `json:"tags"`
}

// BlogArticleSection описывает один смысловой блок статьи.
//
// Level нужен, чтобы фронтенд мог строить содержание без отдельного поля
// `toc_items`.
type BlogArticleSection struct {
	ID      string              `json:"id"`
	Title   string              `json:"title"`
	Level   int                 `json:"level"`
	Kind    BlogSectionKind     `json:"kind"`
	Body    string              `json:"body,omitempty"`
	List    []string            `json:"list,omitempty"`
	Table   *BlogSectionTable   `json:"table,omitempty"`
	Callout *BlogSectionCallout `json:"callout,omitempty"`
	Cards   []BlogSectionCard   `json:"cards,omitempty"`
	Steps   []BlogSectionStep   `json:"steps,omitempty"`
}

// BlogSectionKind помогает фронтенду понять, как рендерить блок статьи.
type BlogSectionKind string

// BlogSectionKind values задают допустимые типы смысловых блоков статьи.
const (
	// BlogSectionKindText описывает обычный текстовый блок.
	BlogSectionKindText BlogSectionKind = "text"
	// BlogSectionKindList описывает блок со списком строк.
	BlogSectionKindList BlogSectionKind = "list"
	// BlogSectionKindTable описывает табличный блок.
	BlogSectionKindTable BlogSectionKind = "table"
	// BlogSectionKindCallout описывает заметку или предупреждение.
	BlogSectionKindCallout BlogSectionKind = "callout"
	// BlogSectionKindCards описывает сетку карточек со свойствами.
	BlogSectionKindCards BlogSectionKind = "cards"
	// BlogSectionKindSteps описывает пошаговый процесс.
	BlogSectionKindSteps BlogSectionKind = "steps"
)

// BlogSectionTable описывает таблицу внутри статьи.
type BlogSectionTable struct {
	Head  []string   `json:"head"`
	Rows  [][]string `json:"rows"`
	Tones [][]string `json:"tones,omitempty"`
}

// BlogSectionCallout описывает выделенный информационный блок.
type BlogSectionCallout struct {
	Tone  string `json:"tone"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

// BlogSectionCard описывает одну карточку в сетке сравнений или обзорных блоков.
type BlogSectionCard struct {
	Title string                   `json:"title"`
	Tone  string                   `json:"tone,omitempty"`
	Meta  []BlogSectionCardMetaRow `json:"meta,omitempty"`
}

// BlogSectionCardMetaRow описывает строку "ключ-значение" внутри карточки.
type BlogSectionCardMetaRow struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// BlogSectionStep описывает один шаг в инструкции внутри статьи.
type BlogSectionStep struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

// RelatedBlogPost описывает карточку связанной статьи под основным материалом.
type RelatedBlogPost struct {
	Slug          string `json:"slug"`
	Title         string `json:"title"`
	CanonicalPath string `json:"canonical_path"`
}
