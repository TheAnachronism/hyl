-- +goose Up
-- external_sport keeps the provider's own activity type. hyl stores eight coarse
-- sport keys, so without this an imported ride of any flavour (gravel, mountain,
-- virtual, e-bike) and anything outside the eight (weight training, yoga, padel)
-- could only ever go back out as a generic activity.
ALTER TABLE activities ADD COLUMN external_sport TEXT;

-- +goose Down
ALTER TABLE activities DROP COLUMN external_sport;
