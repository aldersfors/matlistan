# Family members and recipes in git

You can declare the family and your recipes in the chart values instead of typing them into
the app. Matlistan reads them when it starts, adds and updates them in its database, and
locks them in the app: they show "Hanteras i git" and have no Edit or Archive button, so
every change goes through git.

Recipes the planner creates, recipes you import from a link in the app, and members you add
on the Family page stay editable as before. Each of them can be moved into git later with
"Kopiera som YAML".

## The format

In the chart values (for the homelab: the wrapper's `values.yaml`, under `matlistan:`):

```yaml
household:
  enabled: true
  members:
    - key: anna
      name: Anna
      birthYear: 1985
      diets: [vegetarian]
      allergens: [peanuts]
      likes: "tacos, pasta"
      dislikes: "svamp"
  recipes:
    - key: pumpasoppa
      title: Pumpasoppa
      lang: sv
      servings: 4
      activeMinutes: 20
      totalMinutes: 40
      description: ""
      tags: [soppa]
      diets: [vegetarian]
      allergens: []
      sourceURL: ""
      steps: ["Skala pumpan.", "Koka i 20 minuter."]
      ingredients:
        - {name: pumpa, quantity: 1, unit: kg, section: produce}
        - {name: salt, section: pantry}
        - {name: koriander, quantity: 1, unit: pcs, section: produce, optional: true}
  recipeURLs:
    - https://www.arla.se/recept/pannkaka/
```

Members:

| Field | Required | Notes |
|---|---|---|
| `key` | yes | See "Keys" below |
| `name` | yes | 1 to 60 characters |
| `birthYear` | yes | Used for ages and portions |
| `diets` | no | `vegetarian`, `vegan`, `pescatarian`, `no_pork` |
| `allergens` | no | `gluten`, `crustaceans`, `eggs`, `fish`, `peanuts`, `soybeans`, `milk`, `nuts`, `celery`, `mustard`, `sesame`, `sulphites`, `lupin`, `molluscs` |
| `likes`, `dislikes` | no | Free text, up to 500 characters |

Recipes:

| Field | Required | Notes |
|---|---|---|
| `key` | yes | See "Keys" below |
| `title` | yes | 1 to 120 characters |
| `lang` | no | `sv` or `en`; defaults to the app's language |
| `servings` | yes | 1 to 20 |
| `totalMinutes` | yes | 1 to 1440 |
| `activeMinutes` | no | Up to the total time |
| `description`, `tags`, `diets`, `allergens` | no | As on the recipe form |
| `sourceURL` | no | An https link to where the recipe comes from |
| `steps` | yes | At least one |
| `ingredients` | yes | At least one; see below |

Each ingredient has a `name` and a `section` (`produce`, `dairy`, `meat_fish`, `pantry`,
`frozen`, `bakery`, `other`). Give a `quantity` and a `unit` (`g`, `kg`, `ml`, `dl`, `l`,
`pcs`, `tbsp`, `tsp`, `pinch`) for an amount; leave both out for "efter smak". Mark an
ingredient `optional: true` to show it as "(valfri)".

## Keys

A key is how Matlistan recognises a member or a recipe from one start to the next:
lowercase letters, digits and hyphens, up to 60 characters, unique among members and among
recipes. Renaming a recipe's title keeps its ratings and history because the key stays the
same. Changing a key makes it a different member or recipe.

Every member and recipe in the app already has a key. "Kopiera som YAML" shows it.

## Recipes from links

`recipeURLs` is a list of recipe pages. Matlistan imports each one once, in the background
after it starts, the same way "Importera från länk" does, and locks it. A page is never
imported again once it has a recipe, so restarts cost nothing. If the page redirects, the
recipe keeps the link you wrote.

- A page that cannot be reached is tried again an hour later.
- A page without a recipe, or whose recipe misses something (for example the cooking time),
  is tried again only after the next start. The log says which fields were missing.
- The log names the site, never the full link.

To change a recipe that came from a link, open it, use "Kopiera som YAML", paste it under
`recipes:`, edit it there, and remove the link from `recipeURLs`.

## Moving what you have into git

1. Leave `household.enabled: false` for now.
2. On the Family page and on each recipe you want to keep, open "Kopiera som YAML" and press
   Kopiera. Paste members under `members:` and recipes under `recipes:`.
3. Set `household.enabled: true` and deploy.

The keys in the copied YAML are the ones the app already uses, so nothing is duplicated and
ratings and history stay.

## Removing and bringing back

Remove an entry from the values and it is archived on the next start: it disappears from the
app and from planning, but past weeks and ratings keep it. Add it back with the same key (or
the same link) and it returns with its history. A member who comes back links to their
login again with "Det här är jag".

## Turning it off

Set `household.enabled: false`. On the next start, everything git owned becomes editable in
the app again. Nothing is deleted.

## When it does not start

Matlistan checks the whole file when it starts. If something is wrong, it does not start,
and the pod log lists every problem at once, in the app's language, for example:

```
household.yaml:
  members[anna].name: Obligatoriskt
  recipes[pumpasoppa].total_minutes: Utanför tillåtet intervall
```

The previous version keeps running until the values are fixed. The chart also checks the
structure when it renders, so ArgoCD shows most mistakes before anything is deployed.

## Size

The file is stored as a Kubernetes Secret, which holds at most 1 MiB: enough for hundreds of
recipes.
