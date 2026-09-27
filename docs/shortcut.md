# Shopping list in Apple Notes

Matlistan serves the current shopping list for an iOS Shortcut, which creates a note with a
title, a heading per store section and a real Notes tickbox per item.

Three formats are served, all with the same key:

| Address | Use |
|---|---|
| `current.md` | The checklist shortcut below: headings and tickboxes |
| `current.html` | A note with headings and bullets, through **Make Rich Text from HTML** |
| `current.json` | The title and each section's items as lines, for your own shortcuts |
| `current.txt` | Plain text, for anything else |

## Before you start

- The phone must reach Matlistan: on the home network or through Tailscale.
- If Matlistan uses a certificate from your own CA, install and trust that CA on the phone
  (Settings, General, VPN & Device Management, then About, Certificate Trust Settings).
- In Matlistan, open **Settings**, find **Apple Notes shortcut**, give the device a name and
  press **Create key**. Copy the key; it is shown once. The address is shown next to it.

## Build the shortcut

In the Shortcuts app, create a shortcut with two actions:

1. **Get Contents of URL**
   - URL: the address from Settings, for example
     `https://matlistan.example.org/api/v1/shopping-list/current.md`
   - Method: GET
   - Headers: `Authorization` = `Bearer ` followed by the key
2. **Create Note** with **Contents of URL** as the body, in the folder you want (for example
   a shared "Matlistan" folder, so the whole family sees it). Open its options:
   - **Interpret as Markdown**: on. This is what turns the lines into headings and
     tickboxes.
   - **Name**: empty. The list's first line, "Matlistan vecka 40", becomes the title.

Run it once by hand and check the note: a title, a heading per store section with space
above it, and a tickbox per item.

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
- The note shows `#` and `- [ ]` as text: **Interpret as Markdown** is off in step 2.
- The title appears twice: the **Name** field in step 2 is filled in. Clear it.
- Bullets instead of tickboxes: the shortcut uses `current.html`. Switch to `current.md`.
- A certificate error: the phone does not trust your CA, or it is not on the network or
  Tailscale.

Menu names differ slightly between iOS versions; look for the actions by name.
