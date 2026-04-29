// Move-to-Trash semantics. The agent does not expose a true Trash
// RPC, so we emulate it by `RenameFile`-ing entries into a fixed root
// path. That keeps the operation O(1) when source + destination share
// a filesystem (the typical case for $HOME / / on a sane Linux box)
// and degrades to a clear EXDEV error otherwise.
//
// We chose `/tmp/.platypus-trash` rather than the XDG-standard
// `~/.local/share/Trash/files/` for two reasons:
//   · the agent doesn't surface $HOME, so we'd have to special-case
//     guessing it (`/root` vs `/home/<user>`);
//   · /tmp is reliably writable by every user and is local to one
//     filesystem on most distros, so a rename stays cheap.
//
// The tradeoff is volatility: /tmp may be tmpfs and lose its
// contents at reboot. That's a feature for a recycle-bin-style
// trash — we surface it in the UI as "Move to Trash (cleared on
// reboot)".

export const TRASH_ROOT = "/tmp/.platypus-trash";

// trashTargetPath builds a destination inside TRASH_ROOT for a given
// entry. We prefix with a millisecond timestamp so recovering an old
// version of a file the user trashed twice is still possible — and
// so the rename never collides with a previous trash entry.
export function trashTargetPath(entryName: string, now: number = Date.now()): string {
    // Strip path separators just in case (callers should pass a
    // basename) so a malicious filename can't escape the trash dir.
    const safeName = entryName.replace(/[\\/]/g, "_");
    return `${TRASH_ROOT}/${now}-${safeName}`;
}
