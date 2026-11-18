package routing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type pelicanShowcaseFixture struct {
	PelicanRepository
	settings                     PelicanShowcaseSettings
	saved                        bool
	latestIDs, resultIDs, webIDs []int64
}

func (f *pelicanShowcaseFixture) ShowcaseSettings(context.Context) (PelicanShowcaseSettings, error) {
	return f.settings, nil
}

func (f *pelicanShowcaseFixture) SaveShowcaseSettings(_ context.Context, in PelicanShowcaseSettings) (PelicanShowcaseSettings, error) {
	f.settings, f.saved = in, true
	return in, nil
}

func (f *pelicanShowcaseFixture) Latest(_ context.Context, ids []int64) ([]PelicanLatest, error) {
	f.webIDs = ids
	return []PelicanLatest{{GroupID: 1, QuestionKind: "candy", Answers: []PelicanAnswerPreview{{AnswerPreview: "还剩 20 颗。"}}}}, nil
}

func (f *pelicanShowcaseFixture) LatestDrawings(_ context.Context, ids []int64) ([]PelicanLatest, error) {
	f.latestIDs = ids
	first := PelicanSummary{ID: 9, ModelID: "model", QuestionKind: "pelican", Status: "pending_review", TestedAt: time.Unix(120, 0)}
	return []PelicanLatest{
		{GroupID: 1, Artworks: []PelicanSummary{{ID: 10, ModelID: "another", QuestionKind: "custom_svg", Status: "passed", TestedAt: time.Unix(130, 0)}}},
		{GroupID: 1, Latest: &PelicanSummary{ID: 12, QuestionKind: "custom_text", Status: "passed"}, Artworks: []PelicanSummary{first}},
		{GroupID: 2, Artworks: []PelicanSummary{{ID: 88, ModelID: "hidden", QuestionKind: "custom_svg", Status: "passed", TestedAt: time.Unix(300, 0)}}},
	}, nil
}

func (f *pelicanShowcaseFixture) Result(_ context.Context, id int64, ids []int64) (*PelicanResult, error) {
	f.resultIDs = ids
	sample := PelicanResult{
		PelicanSummary: PelicanSummary{ID: id, QuestionKind: "custom_svg", Status: "pending_review", ModelID: "model", ReasoningEffort: "high"},
		GroupID:        1, Answer: "<svg><circle/></svg>", ProviderName: "account-private", Prompt: "test-private",
	}
	if id == 3 {
		sample.QuestionKind = PelicanStateProbe
	}
	if id == 2 {
		sample.QuestionKind, sample.Answer = "custom_text", "private-text"
	}
	return &sample, nil
}

func TestPelicanShowcaseSettingsValidation(t *testing.T) {
	repo := &pelicanShowcaseFixture{}
	svc := NewPelicanService(repo, PelicanOptions{})
	for _, pair := range [][2]int{{0, 7}, {101, 7}, {20, 0}, {20, 91}, {-1, 7}} {
		_, err := svc.SaveShowcaseSettings(t.Context(), PelicanShowcaseSettings{MaxItems: pair[0], RetentionDays: pair[1]})
		require.ErrorIs(t, err, ErrPelicanInvalid)
		require.False(t, repo.saved)
	}
	for _, pair := range [][2]int{{1, 1}, {100, 90}} {
		input := PelicanShowcaseSettings{Enabled: false, APIEnabled: true, MaxItems: pair[0], RetentionDays: pair[1]}
		saved, err := svc.SaveShowcaseSettings(t.Context(), input)
		require.NoError(t, err)
		require.Equal(t, input, saved)
		visibility, err := svc.Visibility(t.Context())
		require.NoError(t, err)
		require.False(t, visibility.Enabled)
		require.False(t, visibility.APIEnabled)
	}
	repo.settings.MaxItems = -1
	_, err := svc.ShowcaseSettings(t.Context())
	require.Error(t, err)
	_, err = svc.KeyShowcase(t.Context(), 55, []int64{1})
	require.Error(t, err)
	require.Nil(t, repo.latestIDs)
}

func TestPelicanKeyShowcaseUsesOwnerAndKeyGroupIntersection(t *testing.T) {
	repo := &pelicanShowcaseFixture{settings: PelicanShowcaseSettings{Enabled: true, APIEnabled: true, MaxItems: 1, RetentionDays: 7}}
	svc := NewPelicanService(repo, PelicanOptions{UserGroups: func(_ context.Context, id int64) ([]Group, error) {
		require.Equal(t, int64(55), id)
		return []Group{{ID: 1, Name: "one", Status: StatusActive}, {ID: 2, Name: "two", Status: StatusActive}, {ID: 3, Status: StatusDisabled}}, nil
	}})
	scope := []int64{1, 3, 9}
	list, err := svc.KeyShowcase(t.Context(), 55, scope)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, repo.latestIDs)
	require.Nil(t, repo.webIDs)
	require.Len(t, list.Groups, 1)
	require.Equal(t, int64(1), list.Groups[0].ID)
	require.Equal(t, "one", list.Groups[0].Name)
	require.Len(t, list.Groups[0].Items, 1)
	require.Equal(t, int64(10), list.Groups[0].Items[0].ID)
	require.NotNil(t, list.LatestGeneratedAt)
	require.Equal(t, int64(130), list.LatestGeneratedAt.Unix())
	detail, err := svc.KeyArtwork(t.Context(), 55, scope, 1)
	require.NoError(t, err)
	require.Equal(t, []int64{1}, repo.resultIDs)
	require.Equal(t, "<svg><circle/></svg>", detail.ResponseText)
	for _, hiddenID := range []int64{2, 3} {
		_, err = svc.KeyArtwork(t.Context(), 55, scope, hiddenID)
		require.ErrorIs(t, err, ErrPelicanNotFound)
	}
	repo.resultIDs, repo.latestIDs = nil, nil
	for _, scope := range [][]int64{nil, {}, {3, 9}} {
		gallery, err := svc.KeyShowcase(t.Context(), 55, scope)
		require.NoError(t, err)
		require.NotNil(t, gallery.Groups)
		require.Empty(t, gallery.Groups)
		require.Nil(t, gallery.LatestGeneratedAt)
		_, err = svc.KeyArtwork(t.Context(), 55, scope, 1)
		require.ErrorIs(t, err, ErrPelicanNotFound)
	}
	require.Nil(t, repo.resultIDs)
	require.Nil(t, repo.latestIDs)
}

func TestPelicanShowcaseSwitches(t *testing.T) {
	for _, switches := range [][2]bool{{false, false}, {false, true}, {true, false}} {
		repo := &pelicanShowcaseFixture{settings: PelicanShowcaseSettings{Enabled: switches[0], APIEnabled: switches[1], MaxItems: 20, RetentionDays: 7}}
		svc := NewPelicanService(repo, PelicanOptions{UserGroups: func(context.Context, int64) ([]Group, error) {
			return []Group{{ID: 1, Status: StatusActive}}, nil
		}})
		_, err := svc.KeyShowcase(t.Context(), 55, []int64{1})
		require.ErrorIs(t, err, ErrPelicanNotFound)
		_, err = svc.KeyArtwork(t.Context(), 55, []int64{1}, 1)
		require.ErrorIs(t, err, ErrPelicanNotFound)
		require.Nil(t, repo.resultIDs)
		require.Nil(t, repo.latestIDs)
		groups, err := svc.Showcase(t.Context(), 55)
		if switches[0] {
			require.NoError(t, err)
			require.Equal(t, []int64{1}, repo.webIDs)
			require.Equal(t, "还剩 20 颗。", groups[0].Models[0].Answers[0].AnswerPreview)
		} else {
			require.ErrorIs(t, err, ErrPelicanNotFound)
			_, err = svc.UserResult(t.Context(), 55, 1)
			require.ErrorIs(t, err, ErrPelicanNotFound)
			require.Nil(t, repo.resultIDs)
			require.Nil(t, repo.latestIDs)
		}
	}
}
