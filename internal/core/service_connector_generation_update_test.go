package core

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/rmorlok/authproxy/internal/apid"
	"github.com/rmorlok/authproxy/internal/core/mock"
	"github.com/rmorlok/authproxy/internal/database"
	"github.com/rmorlok/authproxy/internal/encfield"
	cschema "github.com/rmorlok/authproxy/internal/schema/resources/connectors"
	"github.com/rmorlok/authproxy/internal/util/pagination"
	"github.com/stretchr/testify/require"
)

// connectorValidationListBuilder supplies the selected connector to update
// validation tests. Unused builder methods remain unsupported.
type connectorValidationListBuilder struct {
	database.ListConnectorsBuilder
	connector database.ConnectorWithDefinition
}

// ForId retains the single connector selected for this validation scenario.
func (b *connectorValidationListBuilder) ForId(apid.ID) database.ListConnectorsBuilder {
	return b
}

// Limit keeps the fixed single-result page used by the service's initial read.
func (b *connectorValidationListBuilder) Limit(int32) database.ListConnectorsBuilder {
	return b
}

// FetchPage permits reads even after cancellation so the test exercises the
// service's validation boundary rather than a database context check.
func (b *connectorValidationListBuilder) FetchPage(context.Context) pagination.PageResult[database.ConnectorWithDefinition] {
	return pagination.PageResult[database.ConnectorWithDefinition]{Results: []database.ConnectorWithDefinition{b.connector}}
}

// TestUpdateConnectorValidationErrors checks both update entry points before
// metadata changes or generation writes can follow canceled validation.
func TestUpdateConnectorValidationErrors(t *testing.T) {
	for _, generation := range []bool{false, true} {
		name := "connector"
		if generation {
			name = "generation"
		}
		t.Run(name, func(t *testing.T) {
			for _, tt := range []struct {
				name       string
				javascript string
				canceled   bool
				wantErr    error
			}{
				{name: "canceled metadata-only validation", canceled: true, wantErr: context.Canceled},
				{name: "canceled javascript validation", javascript: "for (;;) {} // private source", canceled: true, wantErr: context.Canceled},
				{name: "deadline during javascript validation", javascript: "for (;;) {} // private source", wantErr: context.DeadlineExceeded},
				{name: "invalid javascript", javascript: "function ( // private source", wantErr: ErrInvalidArgument},
			} {
				t.Run(tt.name, func(t *testing.T) {
					// Allow only the reads needed to validate; writes and encryption fail the test.
					s, db, _, _, _, e := FullMockService(t, gomock.NewController(t))
					id := apid.New(apid.PrefixConnector)
					existing := database.ConnectorWithDefinition{
						Id: id, Generation: 1, Namespace: "root", Name: "connector",
						State:               database.ConnectorGenerationStateDraft,
						EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "definition"},
					}
					if generation {
						db.EXPECT().GetConnectorGeneration(gomock.Any(), id, uint64(1)).Return(&existing, nil)
					} else {
						db.EXPECT().ListConnectorsBuilder().Return(&connectorValidationListBuilder{connector: existing})
					}
					e.EXPECT().DecryptString(gomock.Any(), existing.EncryptedDefinition).Return(`{"display_name":"Existing"}`, nil)

					// A label change would persist even without a definition, unless cancellation wins.
					patch := cschema.NewConnectorPatch()
					labels := map[string]string{"env": "updated"}
					patch.Metadata.Labels = &labels
					if tt.javascript != "" {
						patch.Spec.Definition = &cschema.ConnectorDefinition{Javascript: tt.javascript}
					}
					ctx, cancel := context.WithCancel(context.Background())
					if tt.wantErr == context.DeadlineExceeded {
						cancel()
						ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
					}
					defer cancel()
					if tt.canceled {
						cancel()
					}

					// Authored syntax errors keep their field path; context failures keep their identity.
					var err error
					if generation {
						_, err = s.UpdateConnectorGeneration(ctx, id, 1, patch)
					} else {
						_, err = s.UpdateConnector(ctx, id, patch)
					}
					require.ErrorIs(t, err, tt.wantErr)
					if tt.wantErr != ErrInvalidArgument {
						require.Equal(t, tt.wantErr, err)
						require.NotErrorIs(t, err, ErrInvalidArgument)
					} else {
						require.ErrorContains(t, err, "$.spec.definition.javascript")
					}
					require.NotContains(t, err.Error(), "private")
				})
			}
		})
	}
}

// TestUpdateConnectorCancellationBeforeDraftValidation covers cancellation
// between validating the selected generation and applying the patch to its draft.
func TestUpdateConnectorCancellationBeforeDraftValidation(t *testing.T) {
	s, db, _, _, _, e := FullMockService(t, gomock.NewController(t))
	id := apid.New(apid.PrefixConnector)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The selected primary validates successfully without changing logical metadata.
	current := database.ConnectorWithDefinition{
		Id: id, Generation: 1, Namespace: "root", Name: "connector",
		State:               database.ConnectorGenerationStatePrimary,
		EncryptedDefinition: encfield.EncryptedField{ID: "dek_test", Data: "primary"},
	}
	db.EXPECT().ListConnectorsBuilder().Return(&connectorValidationListBuilder{connector: current})
	e.EXPECT().DecryptString(gomock.Any(), current.EncryptedDefinition).Return(`{"display_name":"Existing"}`, nil)

	// Cancel while reading the existing draft; no write may follow its validation.
	draft := current
	draft.Generation = 2
	draft.State = database.ConnectorGenerationStateDraft
	draft.EncryptedDefinition.Data = "draft"
	db.EXPECT().GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
		DoAndReturn(func(context.Context, apid.ID, database.ConnectorGenerationState) (*database.ConnectorWithDefinition, error) {
			cancel()
			return &draft, nil
		})
	e.EXPECT().DecryptString(gomock.Any(), draft.EncryptedDefinition).Return(`{"display_name":"Existing"}`, nil)
	patch := cschema.NewConnectorPatch()
	patch.Spec.Definition = &cschema.ConnectorDefinition{DisplayName: "Updated"}

	_, err := s.UpdateConnector(ctx, id, patch)
	require.Equal(t, context.Canceled, err)
	require.NotErrorIs(t, err, ErrInvalidArgument)
}

func TestUpdateDraftConnectorGeneration(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, ac, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testaaaaaaaaaaaa")
		ctx := context.Background()

		// Successful update enqueues an asynq propagation task. The body
		// of the task is opaque to this test; just verify it's submitted.
		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil)

		// Existing draft generation
		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(2)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 2,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
				Labels:     map[string]string{"env": "old"},
			}, nil)

		// Build encrypts
		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		newLabels := map[string]string{"env": "new"}
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 2, database.ConnectorGenerationStateDraft, newLabels, cschema.ConnectorDefinition{
			DisplayName: "Updated",
		}))

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Updated",
		}

		result, err := s.UpdateDraftConnectorGeneration(ctx, id, 2, definition, newLabels, nil)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
		require.Equal(t, "new", result.GetLabels()["env"])
	})

	t.Run("success keeps existing labels when nil", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, ac, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testbbbbbbbbbbbb")
		ctx := context.Background()
		existingLabels := map[string]string{"env": "kept"}

		ac.EXPECT().EnqueueContext(gomock.Any(), gomock.Any()).Return(nil, nil)

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
				Labels:     existingLabels,
			}, nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted-def"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 1, database.ConnectorGenerationStateDraft, existingLabels, cschema.ConnectorDefinition{
			DisplayName: "Test",
		}))

		definition := &cschema.ConnectorDefinition{
			DisplayName: "Test",
		}

		result, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, definition, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "kept", result.GetLabels()["env"])
	})

	t.Run("not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(nil, database.ErrNotFound)

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(nil, errors.New("connection refused"))

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get connector generation")
	})

	t.Run("not draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				State:      database.ConnectorGenerationStatePrimary,
			}, nil)

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotDraft)
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGeneration(gomock.Any(), id, uint64(1)).
			Return(&database.ConnectorWithDefinition{
				Id:         id,
				Generation: 1,
				Namespace:  "root",
				State:      database.ConnectorGenerationStateDraft,
			}, nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "encrypted"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("write failed"))

		_, err := s.UpdateDraftConnectorGeneration(ctx, id, 1, &cschema.ConnectorDefinition{DisplayName: "Test"}, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}

func TestGetOrCreateDraftConnectorGeneration(t *testing.T) {
	t.Run("returns existing draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testcccccccccccc")
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "existing-encrypted-def"}

		existingDef := &cschema.ConnectorDefinition{
			DisplayName: "Existing Draft",
		}
		defJson, _ := json.Marshal(existingDef)

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          2,
				Namespace:           "root",
				State:               database.ConnectorGenerationStateDraft,
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt to verify definition loads
		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(defJson), nil)

		result, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(2), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
	})

	t.Run("creates new draft from latest", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.MustParse("cxr_testdddddddddddd")
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "latest-encrypted-def"}

		latestDef := &cschema.ConnectorDefinition{
			DisplayName: "Latest Generation",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		// No existing draft
		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		// Latest generation
		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          3,
				Namespace:           "root",
				State:               database.ConnectorGenerationStatePrimary,
				Labels:              map[string]string{"env": "prod"},
				EncryptedDefinition: encryptedDef,
			}, nil)

		// Decrypt latest definition
		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		// Build encrypts the new generation
		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted-def"}, nil)

		// Upsert
		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(nil)

		// Re-fetch
		mock.MockConnectorRetrival(ctx, db, e, connectorResourceForMock(id, 4, database.ConnectorGenerationStateDraft, map[string]string{"env": "prod"}, cschema.ConnectorDefinition{
			DisplayName: "Latest Generation",
		}))

		result, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.NoError(t, err)
		require.Equal(t, id, result.GetId())
		require.Equal(t, uint64(4), result.GetGeneration())
		require.Equal(t, database.ConnectorGenerationStateDraft, result.GetState())
		require.Equal(t, "prod", result.GetLabels()["env"])
	})

	t.Run("db error checking for draft", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, errors.New("connection refused"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to check for existing draft")
	})

	t.Run("connector not found", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(nil, database.ErrNotFound)

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("db error getting newest generation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, _ := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(nil, errors.New("timeout"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to get latest connector generation")
	})

	t.Run("upsert error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		s, db, _, _, _, e := FullMockService(t, ctrl)

		id := apid.New(apid.PrefixActor)
		ctx := context.Background()
		encryptedDef := encfield.EncryptedField{ID: "dek_test", Data: "latest-encrypted-def"}

		latestDef := &cschema.ConnectorDefinition{
			DisplayName: "Test",
		}
		latestDefJson, _ := json.Marshal(latestDef)

		db.EXPECT().
			GetConnectorGenerationForState(gomock.Any(), id, database.ConnectorGenerationStateDraft).
			Return(nil, database.ErrNotFound)

		db.EXPECT().
			NewestConnectorGenerationForId(gomock.Any(), id).
			Return(&database.ConnectorWithDefinition{
				Id:                  id,
				Generation:          1,
				Namespace:           "root",
				State:               database.ConnectorGenerationStatePrimary,
				EncryptedDefinition: encryptedDef,
			}, nil)

		e.EXPECT().
			DecryptString(gomock.Any(), encryptedDef).
			Return(string(latestDefJson), nil)

		e.EXPECT().
			EncryptStringForEntity(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(encfield.EncryptedField{ID: "dek_test", Data: "new-encrypted"}, nil)

		db.EXPECT().
			UpsertConnectorGeneration(gomock.Any(), gomock.Any()).
			Return(errors.New("constraint violation"))

		_, err := s.GetOrCreateDraftConnectorGeneration(ctx, id)
		require.Error(t, err)
		require.Contains(t, err.Error(), "failed to upsert connector generation")
	})
}
