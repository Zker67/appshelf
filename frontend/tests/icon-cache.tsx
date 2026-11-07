import { Activity, StrictMode, useState } from "react";
import { createRoot } from "react-dom/client";
import { IconImage } from "../src/components/app-icon";

// This is an isolated component regression fixture, not an app-data preview.
const image = `data:image/svg+xml,${encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="32" height="32"><path fill="blue" d="M0 0h32v32H0z"/></svg>')}`;

function Fixture() {
  const [visible, setVisible] = useState(true);
  const [version, setVersion] = useState(0);
  return (
    <>
      <h1>图标缓存状态回归</h1>
      <button onClick={() => setVisible(false)}>隐藏页面</button>
      <button onClick={() => setVisible(true)}>恢复页面</button>
      <button onClick={() => setVersion((v) => v + 1)}>更换图标 URL</button>
      <p>验证图片解码状态和组件 class，不作视觉验收。</p>
      <Activity mode={visible ? "visible" : "hidden"}>
        <div data-testid="icon">
          <IconImage key={version} baseSrc={`${image}#${version}`} name="Cache regression" />
        </div>
      </Activity>
    </>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <Fixture />
  </StrictMode>,
);
