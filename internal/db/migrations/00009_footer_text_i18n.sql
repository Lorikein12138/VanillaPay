-- +goose Up
UPDATE site_settings
SET footer_legal = jsonb_set(footer_legal, '{items}', (
    SELECT jsonb_agg(
        CASE WHEN jsonb_typeof(item->'text') = 'string'
            THEN jsonb_set(item, '{text}', jsonb_build_object('zh_CN', item->>'text', 'en', ''))
            ELSE item
        END ORDER BY position
    )
    FROM jsonb_array_elements(footer_legal->'items') WITH ORDINALITY AS items(item, position)
))
WHERE jsonb_typeof(footer_legal->'items') = 'array'
    AND footer_legal @? '$.items[*] ? (@.text.type() == "string")';

-- +goose Down
UPDATE site_settings
SET footer_legal = jsonb_set(footer_legal, '{items}', (
    SELECT jsonb_agg(
        CASE WHEN jsonb_typeof(item->'text') = 'object' AND jsonb_typeof(item->'text'->'zh_CN') = 'string'
            THEN jsonb_set(item, '{text}', to_jsonb(item->'text'->>'zh_CN'))
            ELSE item
        END ORDER BY position
    )
    FROM jsonb_array_elements(footer_legal->'items') WITH ORDINALITY AS items(item, position)
))
WHERE jsonb_typeof(footer_legal->'items') = 'array'
    AND footer_legal @? '$.items[*] ? (@.text.type() == "object" && @.text.zh_CN.type() == "string")';
