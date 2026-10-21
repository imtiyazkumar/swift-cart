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
