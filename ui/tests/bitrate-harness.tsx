import { useState } from "react";
import { createRoot } from "react-dom/client";
import { createMemoryRouter, RouterProvider } from "react-router";
import { Vim4BitrateInput } from "../src/components/Vim4BitrateInput";
import "../src/index.css";

function Harness() {
  const [factor, setFactor] = useState(
    Number(localStorage.getItem("test-factor") ?? 17500 / 12000),
  );
  const [busy, setBusy] = useState(false);
  return (
    <div className="p-8">
      <Vim4BitrateInput
        factor={factor}
        disabled={busy}
        onApply={value => {
          setBusy(true);
          void fetch("/test/save", { method: "POST", body: JSON.stringify({ factor: value }) })
            .then(response => {
              if (response.ok) {
                localStorage.setItem("test-factor", String(value));
                setFactor(value);
              }
            })
            .finally(() => setBusy(false));
        }}
      />
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <RouterProvider router={createMemoryRouter([{ path: "/", element: <Harness /> }])} />,
);
