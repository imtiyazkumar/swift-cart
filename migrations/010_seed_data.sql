-- Migration 010: Complete Seed Data for Yember Food Delivery Ecosystem

-- Seed Coupons
INSERT INTO orders.coupons (code, description, discount_type, discount_value, min_order_amount, is_active)
VALUES 
    ('VALLEY20', '20% off on authentic Kashmiri cuisine orders', 'PERCENTAGE', 20, 300, true),
    ('YEMBER50', 'Flat ₹50 off on orders above ₹249', 'FLAT', 50, 249, true)
ON CONFLICT (code) DO NOTHING;

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
