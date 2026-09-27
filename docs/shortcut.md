# Shopping list in Apple Notes

Matlistan serves the current shopping list for an iOS Shortcut, which creates a note with a
title, a heading per store section and a real Notes tickbox per item.

Three formats are served, all with the same key:

| Address | Use |
|---|---|
| `current.json` | The checklist shortcut below. Needs iOS 18.1 or later |
| `current.html` | A note with headings and bullets, through **Make Rich Text from HTML** |
| `current.txt` | Plain text, for anything else |

## Before you start

- The phone must reach Matlistan: on the home network or through Tailscale.
- If Matlistan uses a certificate from your own CA, install and trust that CA on the phone
  (Settings, General, VPN & Device Management, then About, Certificate Trust Settings).
- In Matlistan, open **Settings**, find **Apple Notes shortcut**, give the device a name and
  press **Create key**. Copy the key; it is shown once. The address is shown next to it.

## Build the shortcut

In the Shortcuts app, create a shortcut with these actions. "Get Dictionary Value" takes the
key named in each step.

1. **Get Contents of URL**
   - URL: the address from Settings, for example
     `https://matlistan.example.org/api/v1/shopping-list/current.json`
   - Method: GET
   - Headers: `Authorization` = `Bearer ` followed by the key
2. **Get Dictionary Value** `title` from **Contents of URL**.
3. **Create Note** with **Dictionary Value** as the body, in the folder you want (for example
   a shared "Matlistan" folder, so the whole family sees it). The first line becomes the
   note's title.
4. **Get Dictionary Value** `sections` from **Contents of URL**.
5. **Repeat with Each** item in **Dictionary Value**. Inside the repeat:
   1. **Get Dictionary Value** `name` from **Repeat Item**.
   2. **Append to Note**: an empty line, then **Dictionary Value** on the next line. Note:
      the note from step 3. The empty line is the space between sections.
   3. **Get Dictionary Value** `items` from **Repeat Item**.
   4. **Combine Text** from **Dictionary Value** with **New Lines**.
   5. **Append Checklist Item**: **Combined Text**, to the note from step 3. Each line becomes
      one tickbox.
6. Optional: **Get Dictionary Value** `note` from **Contents of URL**, then **Append to Note**.
   This adds "Allt är köpt." or "Ingen godkänd vecka än." when there is nothing to buy; the
   value is empty otherwise.

Run it once by hand and check the note.

For bold section headings, put **Make Rich Text from HTML** before step 5.2 with
`<br><b>` + **Dictionary Value** + `</b>` and append its result instead.

## Run it every Sunday

In Shortcuts, open **Automation**, add a **Time of Day** automation (for example Sunday
18:00), choose **Run Immediately**, and pick the shortcut. Add a **Show Notification** action
at the end if you want a reminder that the list is ready.

## If something goes wrong

- An empty note, or one that only says `unauthorized`: the key is wrong or was revoked.
  Shortcuts does not stop on this error. Create a new key in Settings and paste it into the
  shortcut. To check the key, run step 1 alone and look at its result.
- The note says everything is bought: every item was ticked in Matlistan.
- The note says there is no approved week: approve the week in Matlistan first.
- The note shows `{"title":...`: step 3 uses **Contents of URL** instead of the dictionary
  value from step 2.
- Bullets instead of tickboxes: the shortcut uses `current.html`. Switch to `current.json`
  and the steps above.
- A certificate error: the phone does not trust your CA, or it is not on the network or
  Tailscale.

Menu names differ slightly between iOS versions; look for the actions by name.
