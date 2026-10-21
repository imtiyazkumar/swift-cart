-- Refresh a few source-specific food photos for the existing demo catalog.
-- These updates are idempotent and leave customer/order data untouched.
WITH food_images(name, image_url, credit, credit_url) AS (
    VALUES
        ('Rista', 'https://images.slurrp.com/prodarticles/gi4jd8e9qu7.webp?impolicy=slurrp-20210601&width=1200&height=900&q=75', NULL::TEXT, NULL::TEXT),
        ('Rogan Josh', 'https://upload.wikimedia.org/wikipedia/commons/6/67/Rogan_Josh_Kashmiri.jpg', 'Spice Eats via Wikimedia Commons, CC BY 3.0', 'https://commons.wikimedia.org/wiki/File:Rogan_Josh_Kashmiri.jpg'),
        ('Steamed Mutton Momos', 'https://images.unsplash.com/photo-1655648316042-1a863d182d29?auto=format&fit=crop&fm=jpg&q=80&w=900', 'Sukanya Basu via Unsplash', 'https://unsplash.com/photos/a-bowl-of-soup-4HEuiIwQ0K4'),
        ('Fried Chicken Momos', 'https://images.unsplash.com/photo-1655648316042-1a863d182d29?auto=format&fit=crop&fm=jpg&q=80&w=900', 'Sukanya Basu via Unsplash', 'https://unsplash.com/photos/a-bowl-of-soup-4HEuiIwQ0K4')
)
UPDATE catalog.items AS item
SET metadata = COALESCE(item.metadata, '{}'::jsonb) || jsonb_strip_nulls(jsonb_build_object(
        'image_url', food_images.image_url,
        'image_credit', food_images.credit,
        'image_credit_url', food_images.credit_url
    )),
    updated_at = now()
FROM food_images
WHERE lower(item.name) = lower(food_images.name)
  AND item.deleted_at IS NULL;
