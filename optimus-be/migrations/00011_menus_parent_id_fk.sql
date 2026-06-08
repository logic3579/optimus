-- +goose Up
ALTER TABLE menus
    ADD CONSTRAINT fk_menus_parent FOREIGN KEY (parent_id)
    REFERENCES menus(id) ON DELETE RESTRICT;

-- +goose Down
ALTER TABLE menus DROP CONSTRAINT fk_menus_parent;
