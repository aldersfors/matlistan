# Shopping list in Apple Notes

Matlistan serves the current shopping list as a small web page. An iOS Shortcut fetches it,
turns it into rich text and creates a note: a title, a bold heading per store section, and a
bulleted item per line with the amount in bold. Shortcuts cannot create checklists in Notes;
see [Tickboxes](#tickboxes) for the one-tap way to turn the bullets into one.

A plain-text version is at `current.txt` for anything that does not handle rich text.

## Before you start

- The phone must reach Matlistan: on the home network or through Tailscale.
- If Matlistan uses a certificate from your own CA, install and trust that CA on the phone
  (Settings, General, VPN & Device Management, then About, Certificate Trust Settings).
- In Matlistan, open **Settings**, find **Apple Notes shortcut**, give the device a name and
  press **Create key**. Copy the key; it is shown once. The address is shown next to it.

## Build the shortcut

In the Shortcuts app, create a shortcut with these actions:

1. **Get Contents of URL**
   - URL: the address from Settings with `.html` instead of `.txt`, for example
     `https://matlistan.example.org/api/v1/shopping-list/current.html`
   - Method: GET
   - Headers: `Authorization` = `Bearer ` followed by the key
2. **Make Rich Text from HTML** with the result of step 1.
3. **Create Note** with the rich text from step 2 as the body, in the folder you want (for
   example a shared "Matlistan" folder, so the whole family sees it).

If you built the shortcut before rich text existed, change the address to `.html` and add
step 2 between the two actions you have.

Run it once by hand and check the note.

## Run it every Sunday

In Shortcuts, open **Automation**, add a **Time of Day** automation (for example Sunday
18:00), choose **Run Immediately**, and pick the shortcut. Add a **Show Notification** action
at the end if you want a reminder that the list is ready.

## Tickboxes

In the note, select the items of a section (or the whole note) and tap the checklist button
in the toolbar. The bullets become tickboxes, and the headings stay as they are.

## If something goes wrong

- The note only says `unauthorized`: the key is wrong or was revoked. Shortcuts does not
  stop on this error, so the word ends up in the note. Create a new key in Settings and
  paste it into the shortcut.
- The note says everything is bought: every item was ticked in Matlistan.
- The note says there is no approved week: approve the week in Matlistan first.
- The note shows raw tags like `<h1>`: step 2 is missing, or step 3 uses the result of step 1
  instead of step 2.
- A certificate error: the phone does not trust your CA, or it is not on the network or
  Tailscale.

Menu names differ slightly between iOS versions; look for the actions by name.
