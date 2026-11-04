import type { Group } from "../api";
import { Folder } from "lucide-react";
import Dropdown from "./Dropdown";

type Props = {
  groups: Group[];
  value: string;
  onChange: (id: string) => void;
};

export default function GroupSelect({ groups, value, onChange }: Props) {
  return (
    <Dropdown
      label="Group"
      optional
      value={value}
      onChange={onChange}
      icon={<Folder size={16} />}
      options={[
        { value: "", label: "No group" },
        ...groups.map((group) => ({ value: group.id, label: group.name })),
      ]}
      hint={
        groups.length
          ? undefined
          : "Create a group from the sidebar to add it here"
      }
    />
  );
}
