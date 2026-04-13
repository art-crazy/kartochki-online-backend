package handlers

import (
	openapi "kartochki-online-backend/api/gen"
	"kartochki-online-backend/internal/blog"
)

func toBlogListResponse(result blog.ListResult) openapi.BlogListResponse {
	responsePayload := openapi.BlogListResponse{
		Posts:        toBlogListItems(result.Posts),
		Categories:   toBlogCategories(result.Categories),
		PopularPosts: toBlogSidebarPosts(result.PopularPosts),
		Tags:         toBlogTags(result.Tags),
		Pagination: openapi.BlogPagination{
			Page:       result.Pagination.Page,
			PageSize:   result.Pagination.PageSize,
			TotalPages: result.Pagination.TotalPages,
		},
	}

	if result.FeaturedPost != nil {
		responsePayload.FeaturedPost = &openapi.FeaturedBlogPost{
			Slug:               result.FeaturedPost.Slug,
			Title:              result.FeaturedPost.Title,
			Excerpt:            result.FeaturedPost.Excerpt,
			PublishedAt:        result.FeaturedPost.PublishedAt,
			ReadingTimeMinutes: result.FeaturedPost.ReadingTimeMinutes,
			CanonicalPath:      blogCanonicalPrefix + result.FeaturedPost.Slug,
		}
	}

	return responsePayload
}

func toBlogPostResponse(post blog.Post) openapi.BlogPostResponse {
	return openapi.BlogPostResponse{
		Post: openapi.BlogPostDetail{
			Slug:               post.Slug,
			Title:              post.Title,
			Description:        post.Description,
			Excerpt:            post.Excerpt,
			CanonicalPath:      blogCanonicalPrefix + post.Slug,
			PublishedAt:        post.PublishedAt,
			UpdatedAt:          post.UpdatedAt,
			ReadingTimeMinutes: post.ReadingTimeMinutes,
			Views:              post.Views,
			Tags:               toBlogTags(post.Tags),
		},
		ArticleSections: toBlogArticleSections(post.Sections),
		RelatedPosts:    toRelatedBlogPosts(post.RelatedPosts),
	}
}

func toBlogListItems(items []blog.ListItem) []openapi.BlogListItem {
	result := make([]openapi.BlogListItem, len(items))
	for i, item := range items {
		result[i] = openapi.BlogListItem{
			Slug:               item.Slug,
			Title:              item.Title,
			Excerpt:            item.Excerpt,
			CategoryLabel:      item.CategoryLabel,
			PublishedAt:        item.PublishedAt,
			ReadingTimeMinutes: item.ReadingTimeMinutes,
			CanonicalPath:      blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toBlogCategories(items []blog.Category) []openapi.BlogCategory {
	result := make([]openapi.BlogCategory, len(items))
	for i, item := range items {
		result[i] = openapi.BlogCategory{
			Slug:  item.Slug,
			Label: item.Label,
			Count: item.Count,
		}
	}
	return result
}

func toBlogSidebarPosts(items []blog.SidebarPost) []openapi.BlogSidebarPost {
	result := make([]openapi.BlogSidebarPost, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSidebarPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toRelatedBlogPosts(items []blog.SidebarPost) []openapi.RelatedBlogPost {
	result := make([]openapi.RelatedBlogPost, len(items))
	for i, item := range items {
		result[i] = openapi.RelatedBlogPost{
			Slug:          item.Slug,
			Title:         item.Title,
			CanonicalPath: blogCanonicalPrefix + item.Slug,
		}
	}
	return result
}

func toBlogTags(items []blog.Tag) []openapi.BlogTag {
	result := make([]openapi.BlogTag, len(items))
	for i, item := range items {
		result[i] = openapi.BlogTag{
			Slug:  item.Slug,
			Label: item.Label,
		}
	}
	return result
}

func toBlogArticleSections(items []blog.ArticleSection) []openapi.BlogArticleSection {
	result := make([]openapi.BlogArticleSection, len(items))
	for i, item := range items {
		id := mustParseUUID(item.ID)

		section := openapi.BlogArticleSection{
			Id:      id,
			Title:   item.Title,
			Level:   item.Level,
			Kind:    openapi.BlogSectionKind(item.Kind),
			Table:   toBlogSectionTable(item.Table),
			Callout: toBlogSectionCallout(item.Callout),
		}
		if item.Body != "" {
			section.Body = &item.Body
		}
		if len(item.List) > 0 {
			list := item.List
			section.List = &list
		}
		if len(item.Cards) > 0 {
			cards := toBlogSectionCards(item.Cards)
			section.Cards = &cards
		}
		if len(item.Steps) > 0 {
			steps := toBlogSectionSteps(item.Steps)
			section.Steps = &steps
		}
		result[i] = section
	}
	return result
}

func toBlogSectionTable(table *blog.SectionTable) *openapi.BlogSectionTable {
	if table == nil {
		return nil
	}
	t := &openapi.BlogSectionTable{
		Head: table.Head,
		Rows: table.Rows,
	}
	if len(table.Tones) > 0 {
		t.Tones = &table.Tones
	}
	return t
}

func toBlogSectionCallout(callout *blog.SectionCallout) *openapi.BlogSectionCallout {
	if callout == nil {
		return nil
	}
	return &openapi.BlogSectionCallout{
		Tone:  callout.Tone,
		Title: callout.Title,
		Text:  callout.Text,
	}
}

func toBlogSectionCards(items []blog.SectionCard) []openapi.BlogSectionCard {
	result := make([]openapi.BlogSectionCard, len(items))
	for i, item := range items {
		card := openapi.BlogSectionCard{
			Title: item.Title,
		}
		if item.Tone != "" {
			card.Tone = &item.Tone
		}
		if len(item.Meta) > 0 {
			meta := toBlogSectionCardMetaRows(item.Meta)
			card.Meta = &meta
		}
		result[i] = card
	}
	return result
}

func toBlogSectionCardMetaRows(items []blog.SectionCardMetaRow) []openapi.BlogSectionCardMetaRow {
	result := make([]openapi.BlogSectionCardMetaRow, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSectionCardMetaRow{
			Label: item.Label,
			Value: item.Value,
		}
	}
	return result
}

func toBlogSectionSteps(items []blog.SectionStep) []openapi.BlogSectionStep {
	result := make([]openapi.BlogSectionStep, len(items))
	for i, item := range items {
		result[i] = openapi.BlogSectionStep{
			Title:       item.Title,
			Description: item.Description,
		}
	}
	return result
}
