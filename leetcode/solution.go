package leetcode

import (
	"fmt"
	"strings"
)

// GetSolutionList fetches list of solutions for a question
func (c *cnClient) GetSolutionList(questionSlug string, skip, limit int) (*SolutionList, error) {
	query := `
    query questionTopicsList($questionSlug: String!, $skip: Int, $first: Int, $orderBy: SolutionArticleOrderBy, $userInput: String, $tagSlugs: [String!]) {
      questionSolutionArticles(
        questionSlug: $questionSlug
        skip: $skip
        first: $first
        orderBy: $orderBy
        userInput: $userInput
        tagSlugs: $tagSlugs
      ) {
        totalNum
        edges {
          node {
            uuid
            title
            slug
            status
            upvoteCount
            favoriteCount
            createdAt
            author {
              username
              profile {
                userSlug
                realName
                userAvatar
              }
            }
            tags {
              name
              nameTranslated
              slug
              tagType
            }
            summary
            topic {
              id
              commentCount
              viewCount
            }
          }
        }
      }
    }
    `

	variables := map[string]any{
		"questionSlug": questionSlug,
		"skip":         skip,
		"first":        limit,
		"orderBy":      "DEFAULT",
		"userInput":    "",
		"tagSlugs":     []string{},
	}

	var result struct {
		Data struct {
			QuestionSolutionArticles SolutionList `json:"questionSolutionArticles"`
		} `json:"data"`
	}

	req := graphqlRequest{
		query:         query,
		operationName: "questionTopicsList",
		variables:     variables,
		authType:      withAuth,
	}

	_, err := c.graphqlPost(req, &result)
	if err != nil {
		return nil, fmt.Errorf("fetch solution list: %w", err)
	}

	return &result.Data.QuestionSolutionArticles, nil
}

// GetSolutionDetail fetches full content of a solution by slug
func (c *cnClient) GetSolutionDetail(slug string) (*Solution, error) {
	query := `
    query discussTopic($slug: String) {
  solutionArticle(slug: $slug, orderBy: DEFAULT) {
    ...solutionArticle
    content
    next {
      slug
      title
    }
    prev {
      slug
      title
    }
  }
}

    fragment solutionArticle on SolutionArticleNode {
  ipRegion
  rewardEnabled
  canEditReward
  uuid
  title
  content
  slateValue
  slug
  sunk
  chargeType
  status
  identifier
  canEdit
  canSee
  reactionType
  reactionsV2 {
    count
    reactionType
  }
  tags {
    name
    nameTranslated
    slug
    tagType
  }
  createdAt
  thumbnail
  author {
    username
    certificationLevel
    isDiscussAdmin
    isDiscussStaff
    profile {
      userAvatar
      userSlug
      realName
      reputation
    }
  }
  summary
  topic {
    id
    subscribed
    commentCount
    viewCount
    post {
      id
      status
      voteStatus
      isOwnPost
    }
  }
  byLeetcode
  isMyFavorite
  isMostPopular
  favoriteCount
  isEditorsPick
  hitCount
  videosInfo {
    videoId
    coverUrl
    duration
  }
  question {
    titleSlug
    questionFrontendId
  }
}
    `

	variables := map[string]any{
		"slug": slug,
	}

	var result struct {
		Data struct {
			SolutionArticle struct {
				SolutionMetadata
				Content  string `json:"content"`
				Question struct {
					TitleSlug          string `json:"titleSlug"`
					QuestionFrontendId string `json:"questionFrontendId"`
				} `json:"question"`
			} `json:"solutionArticle"`
		} `json:"data"`
	}

	req := graphqlRequest{
		query:         query,
		operationName: "discussTopic",
		variables:     variables,
		authType:      withAuth,
	}

	_, err := c.graphqlPost(req, &result)
	if err != nil {
		return nil, fmt.Errorf("fetch solution detail: %w", err)
	}

	article := result.Data.SolutionArticle
	solution := &Solution{
		SolutionMetadata: article.SolutionMetadata,
		Content:          article.Content,
		QuestionSlug:     article.Question.TitleSlug,
		QuestionID:       article.Question.QuestionFrontendId,
	}

	return solution, nil
}

// FilterSolutionsByUsers filters solutions by followed users
func FilterSolutionsByUsers(solutions []SolutionEdge, followedUsers []string) []SolutionEdge {
	if len(followedUsers) == 0 {
		return solutions
	}

	userMap := make(map[string]bool)
	for _, user := range followedUsers {
		userMap[strings.ToLower(user)] = true
	}

	filtered := make([]SolutionEdge, 0)
	for _, edge := range solutions {
		username := strings.ToLower(edge.Node.Author.Username)
		userSlug := strings.ToLower(edge.Node.Author.Profile.UserSlug)
		if userMap[username] || userMap[userSlug] {
			filtered = append(filtered, edge)
		}
	}

	return filtered
}
