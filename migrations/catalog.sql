-- Domain: catalog
-- Applied through cmd/migrate using manifest.json; do not execute this file directly.
-- yember-step: 006_domain_schemas.sql/003

CREATE SCHEMA IF NOT EXISTS catalog;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/016


CREATE TABLE IF NOT EXISTS catalog.categories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE(merchant_id, name)
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/017


CREATE TABLE IF NOT EXISTS catalog.items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES catalog.categories(id) ON DELETE RESTRICT,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    item_type TEXT NOT NULL DEFAULT 'STANDARD',
    is_vegetarian BOOLEAN,
    base_price BIGINT NOT NULL CHECK (base_price >= 0),
    currency CHAR(3) NOT NULL DEFAULT 'INR',
    is_available BOOLEAN NOT NULL DEFAULT true,
    inventory_tracked BOOLEAN NOT NULL DEFAULT false,
    substitution_policy TEXT NOT NULL DEFAULT 'NONE',
    metadata JSONB NOT NULL DEFAULT '{}',
    created_by UUID REFERENCES public.users(id),
    updated_by UUID REFERENCES public.users(id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/018


CREATE TABLE IF NOT EXISTS catalog.item_variants (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    item_id UUID NOT NULL REFERENCES catalog.items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    price BIGINT NOT NULL CHECK (price >= 0),
    sku TEXT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/019


CREATE TABLE IF NOT EXISTS catalog.modifiers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES public.merchants(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    min_select INTEGER NOT NULL DEFAULT 0,
    max_select INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/020


CREATE TABLE IF NOT EXISTS catalog.addons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    modifier_id UUID NOT NULL REFERENCES catalog.modifiers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    price BIGINT NOT NULL DEFAULT 0 CHECK (price >= 0),
    is_active BOOLEAN NOT NULL DEFAULT true
);
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/035


CREATE INDEX IF NOT EXISTS idx_catalog_items_merchant ON catalog.items(merchant_id) WHERE deleted_at IS NULL;
-- yember-end-step
-- yember-step: 006_domain_schemas.sql/036

CREATE INDEX IF NOT EXISTS idx_catalog_items_search ON catalog.items USING gin (to_tsvector('simple', name || ' ' || description));
-- yember-end-step
-- yember-step: 007_inventory_ready.sql/001
-- Removed unused warehouse inventory tables (not needed for Yember Food Delivery)
-- Food availability is managed via is_available flag on catalog.items

-- yember-end-step
-- yember-step: 009_multi_app_ecosystem.sql/001
-- Migration 009: Multi-App Ecosystem Extensions

-- Drop unused inventory schema if present
DROP SCHEMA IF EXISTS inventory CASCADE;
-- yember-end-step
-- yember-step: 010_seed_data.sql/001
-- Migration 010: Complete Seed Data for Yember Food Delivery Ecosystem

-- Seed Coupons
INSERT INTO orders.coupons (code, description, discount_type, discount_value, min_order_amount, is_active)
VALUES 
    ('VALLEY20', '20% off on authentic Kashmiri cuisine orders', 'PERCENTAGE', 20, 300, true),
    ('YEMBER50', 'Flat ₹50 off on orders above ₹249', 'FLAT', 50, 249, true)
ON CONFLICT (code) DO NOTHING;
-- yember-end-step
-- yember-step: 010_seed_data.sql/002


-- Seed Demo Users & Full Restaurant Catalogs
DO $$
DECLARE
    v_admin_id UUID := '00000000-0000-4000-a000-000000000001';
    v_customer_id UUID := '00000000-0000-4000-a000-000000000002';
    v_merchant_user_id UUID := '00000000-0000-4000-a000-000000000003';
    v_rider_user_id UUID := '00000000-0000-4000-a000-000000000004';

    -- Restaurant IDs
    v_m1 UUID := '00000000-0000-4000-a000-000000000010'; -- Ahdoos
    v_m2 UUID := '00000000-0000-4000-a000-000000000011'; -- Mughal Darbar
    v_m3 UUID := '00000000-0000-4000-a000-000000000012'; -- Lhasa
    v_m4 UUID := '00000000-0000-4000-a000-000000000013'; -- Valley Spice
    v_m5 UUID := '00000000-0000-4000-a000-000000000014'; -- The Pizza Cafe
    v_m6 UUID := '00000000-0000-4000-a000-000000000015'; -- Dal View

    v_partner_id UUID := '00000000-0000-4000-a000-000000000020';
    v_cust_record_id UUID := '00000000-0000-4000-a000-000000000030';

    -- Category IDs
    c1_wazwan UUID := gen_random_uuid(); c1_bev UUID := gen_random_uuid();
    c2_biryani UUID := gen_random_uuid(); c2_tandoor UUID := gen_random_uuid();
    c3_tibetan UUID := gen_random_uuid(); c3_chinese UUID := gen_random_uuid();
    c4_tea UUID := gen_random_uuid(); c4_bakery UUID := gen_random_uuid();
    c5_pizza UUID := gen_random_uuid(); c5_sides UUID := gen_random_uuid();
    c6_veg UUID := gen_random_uuid(); c6_thali UUID := gen_random_uuid();
BEGIN
    -- Insert Demo Users
    INSERT INTO public.users (id, email, password_hash, role, phone, full_name)
    VALUES 
        (v_admin_id, 'admin@yember.app', '$2a$10$7R0Zf9Hk6S0v1r2q3s4u5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t', 'ADMIN', '+919900000001', 'Yember Admin'),
        (v_customer_id, 'customer@yember.app', '$2a$10$7R0Zf9Hk6S0v1r2q3s4u5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t', 'CUSTOMER', '+919900000002', 'Aamir Bhat'),
        (v_merchant_user_id, 'restaurant@yember.app', '$2a$10$7R0Zf9Hk6S0v1r2q3s4u5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t', 'MERCHANT_OWNER', '+919900000003', 'Ahdoos Manager'),
        (v_rider_user_id, 'rider@yember.app', '$2a$10$7R0Zf9Hk6S0v1r2q3s4u5e6f7g8h9i0j1k2l3m4n5o6p7q8r9s0t', 'DELIVERY_PARTNER', '+919900000004', 'Tariq Rider')
    ON CONFLICT (email) DO NOTHING;

    -- Insert Customer Record
    INSERT INTO public.customers (id, user_id, email, phone, name)
    VALUES (v_cust_record_id, v_customer_id, 'customer@yember.app', '+919900000002', 'Aamir Bhat')
    ON CONFLICT DO NOTHING;

    -- 1. Ahdoos Restaurant
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m1, v_merchant_user_id, 'ahdoos@yember.app', 'Ahdoos Restaurant', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Wazwan', 'Kashmiri', 'Traditional'])
    ON CONFLICT DO NOTHING;

    -- 2. Mughal Darbar
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m2, v_merchant_user_id, 'mughal@yember.app', 'Mughal Darbar', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Wazwan', 'Biryani', 'Mughlai'])
    ON CONFLICT DO NOTHING;

    -- 3. Lhasa Restaurant
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m3, v_merchant_user_id, 'lhasa@yember.app', 'Lhasa Restaurant', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Tibetan', 'Chinese', 'Asian'])
    ON CONFLICT DO NOTHING;

    -- 4. Valley Spice Cafe
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m4, v_merchant_user_id, 'valleyspice@yember.app', 'Valley Spice Cafe', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Cafe', 'Kashmiri Tea', 'Bakery'])
    ON CONFLICT DO NOTHING;

    -- 5. The Pizza Cafe
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m5, v_merchant_user_id, 'pizzacafe@yember.app', 'The Pizza Cafe', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Pizza', 'Burgers', 'Fast Food'])
    ON CONFLICT DO NOTHING;

    -- 6. Dal View Restaurant
    INSERT INTO public.merchants (id, owner_id, email, name, is_active, merchant_type, onboarding_status, approval_status, tags)
    VALUES (v_m6, v_merchant_user_id, 'dalview@yember.app', 'Dal View Restaurant', true, 'RESTAURANT', 'COMPLETED', 'APPROVED', ARRAY['Vegetarian', 'Kashmiri', 'Indian'])
    ON CONFLICT DO NOTHING;

    -- Insert Merchant Branches
    INSERT INTO merchants.merchant_branches (merchant_id, name, address, latitude, longitude, is_active)
    VALUES 
        (v_m1, 'Residency Road Branch', 'Residency Road, Srinagar', 34.0722, 74.8105, true),
        (v_m2, 'M.A. Road Branch', 'M.A. Road, Srinagar', 34.0740, 74.8120, true),
        (v_m3, 'Boulevard Road Branch', 'Boulevard Road, Srinagar', 34.0810, 74.8320, true),
        (v_m4, 'Lal Chowk Branch', 'Lal Chowk, Srinagar', 34.0710, 74.8090, true),
        (v_m5, 'Hyderpora Branch', 'Bypass Road, Hyderpora, Srinagar', 34.0450, 74.7950, true),
        (v_m6, 'Dal Lake Promenade', 'Boulevard Road, Dal Lake, Srinagar', 34.0850, 74.8380, true)
    ON CONFLICT DO NOTHING;

    -- Clear previous partial items and categories for fresh complete seed
    DELETE FROM catalog.items WHERE merchant_id IN (v_m1, v_m2, v_m3, v_m4, v_m5, v_m6);
    DELETE FROM catalog.categories WHERE merchant_id IN (v_m1, v_m2, v_m3, v_m4, v_m5, v_m6);

    -- 1. Ahdoos Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c1_wazwan, v_m1, 'Traditional Wazwan', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c1_bev, v_m1, 'Beverages & Desserts', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m1, c1_wazwan, 'Rista', 'Minced mutton meatballs cooked in spicy red saffron gravy', false, 45000, true),
        (v_m1, c1_wazwan, 'Gushtaba', 'Velvety minced mutton cooked in flavorful curd gravy', false, 48000, true),
        (v_m1, c1_wazwan, 'Tabak Maaz', 'Crispy fried lamb ribs marinated with spices', false, 52000, true),
        (v_m1, c1_wazwan, 'Rogan Josh', 'Slow-cooked tender mutton in aromatic Kashmiri red gravy', false, 46000, true),
        (v_m1, c1_wazwan, 'Aab Gosht', 'Succulent mutton cooked in milk and cardamom gravy', false, 47000, true),
        (v_m1, c1_bev, 'Kashmiri Kahwa', 'Traditional saffron green tea brewed with cardamom and almonds', true, 12000, true),
        (v_m1, c1_bev, 'Phirni', 'Creamy ground rice pudding flavoured with saffron & pistachios', true, 15000, true);

    -- 2. Mughal Darbar Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c2_biryani, v_m2, 'Special Biryani & Kebabs', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c2_tandoor, v_m2, 'Tandoori Mains', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m2, c2_biryani, 'Mutton Seekh Kebab', 'Flame grilled spiced mutton seekh kebabs', false, 38000, true),
        (v_m2, c2_biryani, 'Kashmiri Mutton Biryani', 'Long grain basmati rice layered with aromatic spices and tender mutton', false, 42000, true),
        (v_m2, c2_tandoor, 'Chicken Kanti', 'Boneless chicken pieces stir fried with tomatoes, onions and green chillies', false, 39000, true),
        (v_m2, c2_tandoor, 'Tandoori Chicken Full', 'Marinated chicken roasted over hot charcoal tandoor', false, 54000, true),
        (v_m2, c2_tandoor, 'Mutton Kanti', 'Juicy mutton cubes tossed with Kashmiri spices and capsicum', false, 44000, true);

    -- 3. Lhasa Restaurant Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c3_tibetan, v_m3, 'Tibetan Specials', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c3_chinese, v_m3, 'Chinese Wok Favorites', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m3, c3_tibetan, 'Steamed Mutton Momos', 'Authentic Himalayan steamed dumplings stuffed with spiced mutton', false, 26000, true),
        (v_m3, c3_tibetan, 'Fried Chicken Momos', 'Crispy fried momos served with fiery spicy red chilli dip', false, 24000, true),
        (v_m3, c3_tibetan, 'Thukpa Noodle Soup', 'Hearty Tibetan noodle soup with fresh vegetables & tender chicken', false, 28000, true),
        (v_m3, c3_chinese, 'Chicken Chilli Gravy', 'Wok-tossed juicy chicken in garlic soya chilli gravy', false, 34000, true),
        (v_m3, c3_chinese, 'Veg Hakka Noodles', 'Stir-fried noodles with crunchy cabbage, bell peppers and scallions', true, 22000, true);

    -- 4. Valley Spice Cafe Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c4_tea, v_m4, 'Artisanal Teas & Coffee', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c4_bakery, v_m4, 'Srinagar Bakery & Sandwiches', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m4, c4_tea, 'Noon Chai (Pink Tea)', 'Traditional salted Kashmiri pink tea served hot', true, 9000, true),
        (v_m4, c4_bakery, 'Kashmiri Shirmal & Bakarkhani', 'Layered sweet bakery bread infused with ghee & sesame seeds', true, 11000, true),
        (v_m4, c4_bakery, 'Girda Chicken Club Sandwich', 'Local tandoori girda bread packed with grilled chicken and herbs', false, 24000, true),
        (v_m4, c4_tea, 'Cardamom Cappuccino', 'Rich espresso with steamed milk and powdered green cardamom', true, 16000, true),
        (v_m4, c4_bakery, 'Walnut Fudge Brownie', 'Decadent dark chocolate brownie loaded with Kashmiri walnuts', true, 18000, true);

    -- 5. The Pizza Cafe Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c5_pizza, v_m5, 'Gourmet Woodfired Pizzas', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c5_sides, v_m5, 'Sides & Shakes', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m5, c5_pizza, 'Woodfired Margherita', 'Classic sourdough pizza topped with fresh mozzarella and basil', true, 34900, true),
        (v_m5, c5_pizza, 'Chicken Tikka Feast Pizza', 'Loaded with char-grilled chicken tikka, onions, and bell peppers', false, 42900, true),
        (v_m5, c5_pizza, 'Spicy Jalapeno & Corn Pizza', 'Crispy pizza with sweet corn, jalapenos and liquid cheese', true, 36900, true),
        (v_m5, c5_sides, 'Garlic Cheese Breadsticks', 'Oven-baked dough sticks stuffed with garlic butter & mozzarella', true, 19900, true),
        (v_m5, c5_sides, 'Classic Cold Coffee Shake', 'Chilled creamy coffee blended with vanilla ice cream', true, 17000, true);

    -- 6. Dal View Restaurant Items
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c6_veg, v_m6, 'Pure Vegetarian Kashmiri Wazwan', 1, true);
    INSERT INTO catalog.categories (id, merchant_id, name, sort_order, is_active) VALUES (c6_thali, v_m6, 'North Indian Thalis', 2, true);

    INSERT INTO catalog.items (merchant_id, category_id, name, description, is_vegetarian, base_price, is_available) VALUES
        (v_m6, c6_veg, 'Kashmiri Dum Aloo', 'Baby potatoes cooked in rich fennel and ginger red gravy', true, 32000, true),
        (v_m6, c6_veg, 'Nadir Monji', 'Crispy fried lotus stem fritters dusted with Kashmiri spices', true, 28000, true),
        (v_m6, c6_veg, 'Paneer Chaman', 'Fresh cottage cheese cubes cooked in aromatic turmeric milk gravy', true, 36000, true),
        (v_m6, c6_veg, 'Haakh Saag', 'Authentic Kashmiri collard greens simmered in mustard oil & spices', true, 26000, true),
        (v_m6, c6_thali, 'Dal Lake Special Veg Thali', 'Full meal served with Dum Aloo, Paneer Chaman, Haakh, Rice & Naan', true, 45000, true);

    -- Insert Delivery Partner Record
    INSERT INTO delivery.delivery_partners (id, user_id, name, phone, status, approval_status)
    VALUES (v_partner_id, v_rider_user_id, 'Tariq Rider', '+919900000004', 'ONLINE', 'APPROVED')
    ON CONFLICT DO NOTHING;

    -- Insert Initial Live Location for Rider
    INSERT INTO delivery.live_locations (partner_id, latitude, longitude)
    VALUES (v_partner_id, 34.0730, 74.8115);

END $$;

-- yember-end-step
-- yember-step: 016_seed_menu_image_urls.sql/001
-- Add externally hosted demo food photography to the seeded menu catalog.
-- URLs are stored in item metadata so they can later be replaced with owned assets.
WITH food_images(name, image_url) AS (
    VALUES
        ('Rista', 'https://images.slurrp.com/prodarticles/gi4jd8e9qu7.webp?impolicy=slurrp-20210601&width=1200&height=900&q=75'),
        ('Gushtaba', 'https://images.hindustantimes.com/rf/image_size_960x540/HT/p2/2020/10/02/Pictures/_b7015fae-04c5-11eb-b559-b799cd4220ef.jpg'),
        ('Tabak Maaz', 'https://static.toiimg.com/photo/https%3A/static.toiimg.com/thumb/msid-86371118%2Cwidth-1280%2Cheight-720%2Cimgsize-28964%2Cresizemode-6%2Coverlay-toi_sw%2Cpt-32%2Cy_pad-600/photo.jpg'),
        ('Rogan Josh', 'https://etoiledukashmir.fr/images/posts/rogan-josh.webp'),
        ('Aab Gosht', 'https://media-assets.swiggy.com/swiggy/image/upload/fl_lossy%2Cf_auto%2Cq_auto%2Cw_300%2Ch_300%2Cc_fit/FOOD_CATALOG/IMAGES/CMS/2026/2/15/64053737-3743-4e3e-aa0b-194138d45700_4c7327d2-4e70-40a5-ad1d-6204091bea14.png_compressed'),
        ('Kashmiri Kahwa', 'https://kashmirinaturals.com/cdn/shop/articles/KASHMIRI_KAHWA_2_f38c346d-3bac-4181-b3a8-32bb41d0b199.jpg?v=1761751603'),
        ('Phirni', 'https://verygoodrecipes.com/images/blogs/my-tryst-with-food-and-travel/kashmiri-phirni-recipe-diwali-sweet-happy-diwali.640x480.jpg'),
        ('Mutton Seekh Kebab', 'https://images.unsplash.com/photo-1599487488170-d11ec9c172f0?auto=format&fit=crop&w=900&q=80'),
        ('Kashmiri Mutton Biryani', 'https://images.unsplash.com/photo-1589302168068-964664d93dc0?auto=format&fit=crop&w=900&q=80'),
        ('Chicken Kanti', 'https://media-assets.swiggy.com/swiggy/image/upload/fl_lossy%2Cf_auto%2Cq_auto%2Cw_400/RX_THUMBNAIL/IMAGES/VENDOR/2024/5/18/c4ff5e57-f639-49f2-9c15-27a65ac1e373_771622.jpg'),
        ('Tandoori Chicken Full', 'https://media-assets.swiggy.com/swiggy/image/upload/fl_lossy%2Cf_auto%2Cq_auto%2Cw_400/RX_THUMBNAIL/IMAGES/VENDOR/2024/5/18/c4ff5e57-f639-49f2-9c15-27a65ac1e373_771622.jpg'),
        ('Mutton Kanti', 'https://images.unsplash.com/photo-1599487488170-d11ec9c172f0?auto=format&fit=crop&w=900&q=80'),
        ('Steamed Mutton Momos', 'https://media-assets.swiggy.com/swiggy/image/upload/fl_lossy%2Cf_auto%2Cq_auto/13f92c7bedcb61ffd1483f97196abf3a'),
        ('Fried Chicken Momos', 'https://www.swagatam.site/uploads/momo1.jpg'),
        ('Thukpa Noodle Soup', 'https://images.unsplash.com/photo-1569718212165-3a8278d5f624?auto=format&fit=crop&w=900&q=80'),
        ('Chicken Chilli Gravy', 'https://images.unsplash.com/photo-1603894584373-5ac82b2ae398?auto=format&fit=crop&w=900&q=80'),
        ('Veg Hakka Noodles', 'https://images.unsplash.com/photo-1512058564366-18510be2db19?auto=format&fit=crop&w=900&q=80'),
        ('Noon Chai (Pink Tea)', 'https://www.kashmiribazaar.in/image/cache/catalog/pictures/products/Tea/noon-chai-500x445.jpg'),
        ('Kashmiri Shirmal & Bakarkhani', 'https://cdn.arabsstock.com/uploads/images/279595/bakeries-and-ovens-in-the-thumbnail-279595.webp'),
        ('Girda Chicken Club Sandwich', 'https://images.unsplash.com/photo-1528735602780-2552fd46c7af?auto=format&fit=crop&w=900&q=80'),
        ('Cardamom Cappuccino', 'https://images.unsplash.com/photo-1570968915860-54d5c301fa9f?auto=format&fit=crop&w=900&q=80'),
        ('Walnut Fudge Brownie', 'https://www.ngy.co.id/wp-content/uploads/2024/11/Resep-Fudgy-Brownies-yang-Nyoklat-dan-Lumer-di-Mulut.jpg'),
        ('Woodfired Margherita', 'https://images.unsplash.com/photo-1664309641932-0e03e0771b97?auto=format&fit=crop&w=900&q=80'),
        ('Chicken Tikka Feast Pizza', 'https://images.unsplash.com/photo-1664309641932-0e03e0771b97?auto=format&fit=crop&w=900&q=80'),
        ('Spicy Jalapeno & Corn Pizza', 'https://images.unsplash.com/photo-1664309641932-0e03e0771b97?auto=format&fit=crop&w=900&q=80'),
        ('Garlic Cheese Breadsticks', 'https://images.unsplash.com/photo-1571407970349-bc81e7e96d47?auto=format&fit=crop&w=900&q=80'),
        ('Classic Cold Coffee Shake', 'https://images.unsplash.com/photo-1570968915860-54d5c301fa9f?auto=format&fit=crop&w=900&q=80'),
        ('Kashmiri Dum Aloo', 'https://images.unsplash.com/photo-1565557623262-b51c2513a641?auto=format&fit=crop&w=900&q=80'),
        ('Nadir Monji', 'https://images.unsplash.com/photo-1601050690597-df0568f70950?auto=format&fit=crop&w=900&q=80'),
        ('Paneer Chaman', 'https://images.unsplash.com/photo-1603894584373-5ac82b2ae398?auto=format&fit=crop&w=900&q=80'),
        ('Haakh Saag', 'https://images.unsplash.com/photo-1547592180-85f173990554?auto=format&fit=crop&w=900&q=80'),
        ('Dal Lake Special Veg Thali', 'https://b.zmtcdn.com/data/pictures/5/19179345/0c6edff360de0a07017370f52559565f_featured_v2.jpg')
)
UPDATE catalog.items AS item
SET metadata = COALESCE(item.metadata, '{}'::jsonb) || jsonb_build_object('image_url', food_images.image_url),
    updated_at = now()
FROM food_images
WHERE lower(item.name) = lower(food_images.name)
  AND item.deleted_at IS NULL;

-- yember-end-step
-- yember-step: 020_refresh_seed_food_image_urls.sql/001
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

-- yember-end-step
