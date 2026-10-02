-- Made-up household for local development and screenshots. It wipes the app's tables
-- first, so it refuses to run unless psql is given -v seed_dev=1 (mise run dev:seed does).
\if :{?seed_dev}
\else
  \echo 'hack/seed.sql: refusing to run without -v seed_dev=1; it truncates every table'
  \quit
\endif
TRUNCATE ratings, shopping_items, shopping_lists, plan_entries, week_plans,
    recipe_ingredients, recipes, staples, members, api_tokens RESTART IDENTITY CASCADE;

-- Each key is keys.Slug of the name or title, as the app would make it. In English, the
-- app's default locale, so screenshots for the README read in English.
INSERT INTO members (key, name, birth_year, allergens, diets, likes, dislikes) VALUES
    ('anna', 'Anna', 1985, '{}', '{}', 'Thai food, soups', 'coriander'),
    ('erik', 'Erik', 1984, '{}', '{}', 'stews', ''),
    ('maja', 'Maja', 2014, '{nuts}', '{}', 'tacos, pasta', 'mushrooms'),
    ('leo', 'Leo', 2019, '{}', '{}', 'meatballs', 'spicy food');

INSERT INTO staples (name, name_key) VALUES ('Salt', 'salt'), ('Olive oil', 'olive oil'),
    ('Flour', 'flour');

INSERT INTO recipes (key, title, title_key, lang, servings, active_minutes, total_minutes, tags,
    steps, diets, allergens, source) VALUES
    ('creamy-pumpkin-soup', 'Creamy pumpkin soup', 'creamy pumpkin soup', 'en', 4, 15, 30, '{soup}',
     '{"Peel and dice the pumpkin and the onion.","Soften the onion in oil, add the pumpkin and water.","Simmer for 20 minutes and blend smooth with the cream."}',
     '{vegetarian}', '{milk}', 'manual'),
    ('meatballs-with-mashed-potatoes', 'Meatballs with mashed potatoes', 'meatballs with mashed potatoes', 'en', 4, 30, 40, '{favourite}',
     '{"Mix breadcrumbs and milk and let them swell.","Stir in the mince and onion and roll the meatballs.","Fry the meatballs in butter for 10 min.","Boil the potatoes for 20 minutes and mash with milk and butter."}',
     '{}', '{milk,gluten}', 'manual'),
    ('chicken-pasta-with-spinach', 'Chicken pasta with spinach', 'chicken pasta with spinach', 'en', 4, 15, 20, '{weeknight}',
     '{"Cook the pasta.","Fry the chicken, add the spinach and cream.","Toss with the pasta."}',
     '{}', '{milk,gluten}', 'manual'),
    ('pea-soup-and-pancakes', 'Pea soup and pancakes', 'pea soup and pancakes', 'en', 4, 20, 40, '{thursday}',
     '{"Heat the pea soup.","Whisk the batter and fry the pancakes."}',
     '{}', '{milk,eggs,gluten,mustard}', 'manual');

INSERT INTO recipe_ingredients (recipe_id, position, name, quantity, unit, section, optional)
VALUES
    (1, 0, 'pumpkin', 1.5, 'kg', 'produce', false), (1, 1, 'yellow onion', 1, 'pcs', 'produce', false),
    (1, 2, 'whipping cream', 1, 'dl', 'dairy', false), (1, 3, 'salt', 0, '', 'pantry', false),
    (2, 0, 'beef and pork mince', 500, 'g', 'meat_fish', false), (2, 1, 'floury potatoes', 1, 'kg', 'produce', false),
    (2, 2, 'milk', 3, 'dl', 'dairy', false), (2, 3, 'lingonberry jam', 0, '', 'pantry', true),
    (3, 0, 'pasta', 400, 'g', 'pantry', false), (3, 1, 'chicken breast', 600, 'g', 'meat_fish', false),
    (3, 2, 'baby spinach', 130, 'g', 'produce', false), (3, 3, 'whipping cream', 2, 'dl', 'dairy', false),
    (4, 0, 'yellow pea soup', 2, 'pcs', 'pantry', false), (4, 1, 'milk', 6, 'dl', 'dairy', false),
    (4, 2, 'eggs', 3, 'pcs', 'dairy', false), (4, 3, 'mustard', 0, '', 'pantry', true);
