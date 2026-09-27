-- Made-up household for local development and screenshots. It wipes the app's tables
-- first, so it refuses to run unless psql is given -v seed_dev=1 (mise run dev:seed does).
\if :{?seed_dev}
\else
  \echo 'hack/seed.sql: refusing to run without -v seed_dev=1; it truncates every table'
  \quit
\endif
TRUNCATE ratings, shopping_items, shopping_lists, plan_entries, week_plans,
    recipe_ingredients, recipes, staples, members, api_tokens RESTART IDENTITY CASCADE;

INSERT INTO members (name, birth_year, allergens, diets, likes, dislikes) VALUES
    ('Anna', 1985, '{}', '{}', 'thailändskt, soppor', 'koriander'),
    ('Erik', 1984, '{}', '{}', 'grytor', ''),
    ('Maja', 2014, '{nuts}', '{}', 'tacos, pasta', 'svamp'),
    ('Leo', 2019, '{}', '{}', 'köttbullar', 'stark mat');

INSERT INTO staples (name, name_key) VALUES ('Salt', 'salt'), ('Olivolja', 'olivolja'),
    ('Vetemjöl', 'vetemjöl');

INSERT INTO recipes (title, title_key, lang, servings, active_minutes, total_minutes, tags,
    steps, diets, allergens, source) VALUES
    ('Krämig pumpasoppa', 'krämig pumpasoppa', 'sv', 4, 15, 30, '{soppa}',
     '{"Skala och tärna pumpan och löken.","Fräs löken i olja, lägg i pumpan och vatten.","Koka i 20 minuter och mixa slät med grädden."}',
     '{vegetarian}', '{milk}', 'manual'),
    ('Köttbullar med potatismos', 'köttbullar med potatismos', 'sv', 4, 30, 40, '{favorit}',
     '{"Blanda ströbröd och mjölk, låt svälla.","Rör ner färs och lök och rulla bullar.","Stek bullarna i smör 10 min.","Koka potatisen i 20 minuter och mosa med mjölk och smör."}',
     '{}', '{milk,gluten}', 'manual'),
    ('Kycklingpasta med spenat', 'kycklingpasta med spenat', 'sv', 4, 15, 20, '{vardag}',
     '{"Koka pastan.","Stek kycklingen, lägg i spenat och grädde.","Blanda med pastan."}',
     '{}', '{milk,gluten}', 'manual'),
    ('Ärtsoppa och pannkakor', 'ärtsoppa och pannkakor', 'sv', 4, 20, 40, '{torsdag}',
     '{"Värm ärtsoppan.","Vispa smeten och grädda pannkakorna."}',
     '{}', '{milk,eggs,gluten,mustard}', 'manual');

INSERT INTO recipe_ingredients (recipe_id, position, name, quantity, unit, section, optional)
VALUES
    (1, 0, 'pumpa', 1.5, 'kg', 'produce', false), (1, 1, 'gul lök', 1, 'pcs', 'produce', false),
    (1, 2, 'vispgrädde', 1, 'dl', 'dairy', false), (1, 3, 'salt', 0, '', 'pantry', false),
    (2, 0, 'blandfärs', 500, 'g', 'meat_fish', false), (2, 1, 'mjölig potatis', 1, 'kg', 'produce', false),
    (2, 2, 'mjölk', 3, 'dl', 'dairy', false), (2, 3, 'lingonsylt', 0, '', 'pantry', true),
    (3, 0, 'pasta', 400, 'g', 'pantry', false), (3, 1, 'kycklingfilé', 600, 'g', 'meat_fish', false),
    (3, 2, 'babyspenat', 130, 'g', 'produce', false), (3, 3, 'vispgrädde', 2, 'dl', 'dairy', false),
    (4, 0, 'gul ärtsoppa', 2, 'pcs', 'pantry', false), (4, 1, 'mjölk', 6, 'dl', 'dairy', false),
    (4, 2, 'ägg', 3, 'pcs', 'dairy', false), (4, 3, 'senap', 0, '', 'pantry', true);
