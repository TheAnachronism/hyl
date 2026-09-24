// Package social holds the viewer-relative rules and the Strava-like social
// layer: visibility, follows, likes, comments, mentions and notifications.
package social

// VisibilityAllows is the single source of truth for who may see an activity.
//
// viewerID is 0 for an anonymous request. activityVisibility is the activity's
// own override ("default", "everyone", "followers", "only_me"); ownerDefault is
// the owner's account-wide activities_visibility ("everyone", "followers",
// "only_me").
//
// The identical predicate is embedded in the SQL of ListFeedActivities,
// ListUserActivities and GetActivityTotalsBySport; the two must keep agreeing,
// which TestVisibilityAgreesWithSQL asserts over a fixture matrix.
func VisibilityAllows(viewerID, ownerID int64, isAcceptedFollower bool, activityVisibility, ownerDefault string) bool {
	if viewerID != 0 && viewerID == ownerID {
		return true
	}
	switch effectiveVisibility(activityVisibility, ownerDefault) {
	case "everyone":
		return true
	case "followers":
		return isAcceptedFollower
	default: // only_me
		return false
	}
}

// effectiveVisibility resolves an activity's "default" (and any unknown value)
// to the owner's account setting.
func effectiveVisibility(activityVisibility, ownerDefault string) string {
	switch activityVisibility {
	case "everyone", "followers", "only_me":
		return activityVisibility
	default:
		return ownerDefault
	}
}
