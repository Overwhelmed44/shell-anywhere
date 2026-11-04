import { isDesktop } from "./api";
import {
  ClipboardGetText,
  ClipboardSetText,
} from "../wailsjs/runtime/runtime";

export async function copyText(text: string): Promise<void> {
  if (isDesktop) {
    if (!(await ClipboardSetText(text))) {
      throw new Error("Could not copy text");
    }

    return;
  }

  await navigator.clipboard.writeText(text);
}

export async function readClipboardText(): Promise<string> {
  if (isDesktop) {
    return ClipboardGetText();
  }

  return navigator.clipboard.readText();
}
