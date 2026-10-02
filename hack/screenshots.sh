#!/usr/bin/env bash
# Screenshots of every screen at phone size, light and dark, for the visual pass. Needs the
# dev database (seeded), the dev IdP and `mise run dev` running on :8080.
set -euo pipefail
out=.dev/screenshots
mkdir -p "$out"
base=http://localhost:8080
db() { podman exec matlistan-db psql -U matlistan -qtAc "$1"; }

# A planned week: the upcoming ISO week, dinners Monday to Friday with Tuesday's locked, the
# weekend away, then approved with a list. Planning must be configured (any provider and key)
# or the week says it is not set up. The README's images go to docs/screenshots.
read -r y w < <(date -v+7d +"%G %V" 2>/dev/null || date -d '+7 days' +"%G %V")
w=$((10#$w))
jar=$(mktemp)
curl -s -c "$jar" -b "$jar" -L -o /dev/null "$base/week"
curl -s -c "$jar" -b "$jar" -o /dev/null -d "y=$y&w=$w&days.0.home=on&days.1.home=on&days.2.home=on&days.3.home=on&days.4.home=on" "$base/week/context"
db "INSERT INTO plan_entries (plan_id, day, recipe_id, servings, why)
    SELECT p.id, v.d, v.r, 4, v.why FROM week_plans p,
    (VALUES (1, 1, 'Pumpkin is in season and you have not had soup in five weeks.'),
            (2, 2, 'All four gave it top marks in August.'),
            (3, 3, 'Ready in 20 minutes, before handball practice.'),
            (4, 4, 'Pea soup on Thursday, as usual.'),
            (5, 3, 'Friday: quick, and everyone finished it last time.')) v(d, r, why)
    WHERE p.iso_year = $y AND p.iso_week = $w ON CONFLICT DO NOTHING"

curl -s -c "$jar" -b "$jar" -o /dev/null -d "y=$y&w=$w&day=2&locked=1" "$base/week/lock"

shoot() { # scheme name=path...
  node hack/shoot.mjs "$out" "$base" "$@"
}
readme() { # scheme name=path...: one phone screen each, for the README
  SHOOT_FRAME=1 node hack/shoot.mjs docs/screenshots "$base" "$@"
}

for scheme in light dark; do
  shoot "$scheme" "week-draft=/week?y=$y&w=$w" "recipe=/recipes/2?servings=4&day=2" \
    "cook=/recipes/2/cook?servings=4&day=2" \
    "cook-step3=/recipes/2/cook?servings=4&day=2!for(let i=0;i<2;i++)document.getElementById('cook-next').click()" \
    family=/family recipes=/recipes settings=/settings
  readme "$scheme" "week=/week?y=$y&w=$w" "recipe=/recipes/2?servings=4&day=2" \
    "cook=/recipes/2/cook?servings=4&day=2!for(let i=0;i<2;i++)document.getElementById('cook-next').click()"
done
curl -s -c "$jar" -b "$jar" -o /dev/null -d "y=$y&w=$w" "$base/week/approve"
for scheme in light dark; do
  shoot "$scheme" "week-approved=/week?y=$y&w=$w" shopping=/shopping "rate=/week/rate?y=$y&w=$w"
  readme "$scheme" shopping=/shopping
done
rm -f "$jar"

# The README banner, from hack/banner.html and the screenshots above. Served over http, not
# file://, so Chrome loads its fonts.
python3 -m http.server 8099 --bind 127.0.0.1 >/dev/null 2>&1 &
srv=$!
trap 'kill $srv 2>/dev/null' EXIT
sleep 1
SHOOT_FRAME=1 SHOOT_SIZE=1280x640 node hack/shoot.mjs docs http://127.0.0.1:8099 dark \
  banner=/hack/banner.html
mv docs/banner-dark.png docs/banner.png
echo "screenshots in $out"
