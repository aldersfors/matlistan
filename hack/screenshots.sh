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
    (VALUES (1, 1, 'Pumpan är i säsong och ni har inte ätit soppa på fem veckor.'),
            (2, 2, 'Alla fyra gav den Gott! i augusti.'),
            (3, 3, 'Klar på 20 minuter före handbollen.'),
            (4, 4, 'Torsdag som vanligt.'),
            (5, 3, 'Fredag: snabbt, och alla åt upp sist.')) v(d, r, why)
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
echo "screenshots in $out"
