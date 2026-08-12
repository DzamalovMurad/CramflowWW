import { chromium } from 'playwright';
const OUT='/tmp/claude-0/-home-user-CramflowWW/ecd5a8df-8836-5a76-9762-cb9524b5b25c/scratchpad/shots';
const b=await chromium.launch({executablePath:'/opt/pw-browsers/chromium'});
for (const [tag,w] of [['360',360],['375',375],['430',430]]) {
  const page=await (await b.newContext({viewport:{width:w,height:860},deviceScaleFactor:2})).newPage();
  await page.goto('http://localhost:5204/',{waitUntil:'networkidle'}); await page.waitForTimeout(4500);
  await page.screenshot({path:`${OUT}/hero-${tag}.png`});
  if (tag==='375') {
    await page.locator('button[aria-label="сменить тему"]').first().click(); await page.waitForTimeout(900);
    await page.screenshot({path:`${OUT}/hero-dark.png`});
  }
  const r=await page.evaluate(()=>({over:document.documentElement.scrollWidth>document.documentElement.clientWidth+1,
    lines:document.querySelector('.hero-title')?.getClientRects().length}));
  console.log(`${tag}px  выезд: ${r.over?'ДА':'нет'} · строк в заголовке: ${r.lines}`);
}
await b.close();
