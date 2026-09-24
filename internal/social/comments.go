package social

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/markbeep/hyl/internal/api"
	"github.com/markbeep/hyl/internal/apperr"
	"github.com/markbeep/hyl/internal/dto"
	"github.com/markbeep/hyl/internal/reqctx"
)

// maxCommentLength mirrors the documented comment limit.
const maxCommentLength = 1000

// PostComment stores a comment, resolves its mentions and notifies the people
// who are allowed to see them.
func (h *Handlers) PostComment(c echo.Context) error {
	viewer, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	activityID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := c.Bind(&req); err != nil {
		return apperr.ErrInvalidRequest
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return apperr.BadRequest("a comment cannot be empty")
	}
	if len(body) > maxCommentLength {
		return apperr.BadRequest("a comment must be at most %d characters", maxCommentLength)
	}

	ctx := c.Request().Context()
	activity, err := h.visibleActivity(ctx, activityID, viewer.ID)
	if err != nil {
		return err
	}

	comment, err := h.Q.CreateComment(ctx, activityID, viewer.ID, body, time.Now().Unix())
	if err != nil {
		return err
	}

	if err := h.notify(ctx, activity.UserID, viewer.ID, KindComment, &activityID, &comment.ID); err != nil {
		return err
	}

	mentions, dropped, err := h.resolveMentions(ctx, activityID, comment.ID, viewer.ID, body)
	if err != nil {
		return err
	}

	created := api.Comment{
		ID:          comment.ID,
		Username:    viewer.Username,
		DisplayName: viewer.DisplayName,
		Body:        comment.Body,
		Mentions:    mentions,
		CreatedAt:   api.Timestamp(comment.CreatedAt),
		CanDelete:   true,
	}
	if avatarID, err := h.avatarID(ctx, viewer.ID); err == nil {
		created.AvatarURL = dto.AvatarURL(avatarID)
	}
	return c.JSON(http.StatusCreated, api.CommentCreated{Comment: created, DroppedMentions: dropped})
}

// ListComments returns one page of comments, newest first.
func (h *Handlers) ListComments(c echo.Context) error {
	activityID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()
	if _, err := h.visibleActivity(ctx, activityID, reqctx.UserID(c)); err != nil {
		return err
	}

	rows, err := h.Q.ListComments(ctx, activityID, dto.Before(c), dto.Limit(c))
	if err != nil {
		return err
	}

	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	mentions, err := h.mentionsByComment(ctx, ids)
	if err != nil {
		return err
	}

	viewerID := reqctx.UserID(c)
	items := make([]api.Comment, 0, len(rows))
	for _, row := range rows {
		commentMentions := mentions[row.ID]
		if commentMentions == nil {
			commentMentions = []string{}
		}
		items = append(items, api.Comment{
			ID:          row.ID,
			Username:    row.Username,
			DisplayName: row.DisplayName,
			AvatarURL:   dto.AvatarURL(row.AvatarMediaID),
			Body:        row.Body,
			Mentions:    commentMentions,
			CreatedAt:   api.Timestamp(row.CreatedAt),
			CanDelete:   viewerID != 0 && viewerID == row.UserID,
		})
	}
	page := api.CommentPage{Items: items}
	if len(items) > 0 {
		page.NextBefore = dto.NextBefore(items[len(items)-1].ID, int64(len(items)), dto.Limit(c))
	}
	return c.JSON(http.StatusOK, page)
}

// DeleteComment soft-deletes a comment the caller wrote.
func (h *Handlers) DeleteComment(c echo.Context) error {
	viewer, err := reqctx.RequireUser(c)
	if err != nil {
		return err
	}
	commentID, err := dto.IDParam(c, "id")
	if err != nil {
		return err
	}
	ctx := c.Request().Context()

	comment, err := h.Q.GetComment(ctx, commentID)
	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound("no such comment")
	}
	if err != nil {
		return err
	}
	if comment.UserID != viewer.ID {
		return apperr.Forbidden("only the author can delete a comment")
	}
	now := time.Now().Unix()
	if _, err := h.Q.SoftDeleteComment(ctx, &now, commentID); err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// resolveMentions stores the mentions whose policy allows them and reports the
// usernames that were refused.
func (h *Handlers) resolveMentions(ctx context.Context, activityID, commentID, authorID int64, body string) (allowed []string, dropped []string, err error) {
	allowed = []string{}
	dropped = []string{}
	for _, username := range ParseMentions(body) {
		mentioned, err := h.Q.GetUserByUsername(ctx, username)
		if errors.Is(err, sql.ErrNoRows) {
			dropped = append(dropped, username)
			continue
		}
		if err != nil {
			return nil, nil, err
		}

		followsAuthor := false
		follow, err := h.Q.GetFollow(ctx, mentioned.ID, authorID)
		switch {
		case err == nil:
			followsAuthor = follow.Status == "accepted"
		case errors.Is(err, sql.ErrNoRows):
		default:
			return nil, nil, err
		}
		if !MentionPolicyAllows(followsAuthor, mentioned.MentionPolicy) {
			dropped = append(dropped, mentioned.Username)
			continue
		}

		if err := h.Q.CreateMention(ctx, commentID, mentioned.ID); err != nil {
			return nil, nil, err
		}
		if err := h.notify(ctx, mentioned.ID, authorID, KindMention, &activityID, &commentID); err != nil {
			return nil, nil, err
		}
		allowed = append(allowed, mentioned.Username)
	}
	return allowed, dropped, nil
}

func (h *Handlers) mentionsByComment(ctx context.Context, commentIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(commentIDs))
	if len(commentIDs) == 0 {
		return out, nil
	}
	rows, err := h.Q.ListMentionsForComments(ctx, commentIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.CommentID] = append(out[row.CommentID], row.Username)
	}
	return out, nil
}

func (h *Handlers) avatarID(ctx context.Context, userID int64) (int64, error) {
	avatar, err := h.Q.GetActiveAvatar(ctx, userID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return avatar.ID, nil
}
