import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { errorText } from "./utils";

export function useSavedPassword(
  kind: "host" | "identity",
  id: string,
  workspaceId = "",
) {
  const source = `${kind}:${workspaceId}:${id}`;
  const [stored, setStored] = useState({ source: "", password: "", error: "" });
  const editRevision = useRef(0);

  useEffect(() => {
    if (!id) return;
    let active = true;
    const revision = editRevision.current;
    const request =
      kind === "host"
        ? api.getHostPassword(id)
        : api.getIdentityPassword(id);
    void request
      .then((password) => {
        if (active && editRevision.current === revision)
          setStored({ source, password, error: "" });
      })
      .catch((reason) => {
        if (active && editRevision.current === revision)
          setStored({ source, password: "", error: errorText(reason) });
      });
    return () => {
      active = false;
    };
  }, [kind, id, source]);

  const current = stored.source === source;
  return {
    password: current ? stored.password : "",
    passwordError: current ? stored.error : "",
    loadingPassword: Boolean(id) && !current,
    setPassword: (password: string) => {
      editRevision.current++;
      setStored({ source, password, error: "" });
    },
  };
}
