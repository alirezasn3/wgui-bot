import qr from "npm:qrcode";
import { createCanvas } from "https://deno.land/x/canvas@v1.4.2/mod.ts";

if (import.meta.main) {
  const config = `[Interface]\nPrivateKey=${Deno.args[0]}\nAddress=${Deno.args[1]
    }\nDNS=1.1.1.1,8.8.8.8\n[Peer]\nPublicKey=${Deno.args[2]
    }\nAllowedIPs=0.0.0.0/0\nEndpoint=${Deno.args[3]}`;
  const canvas = createCanvas(720 * 2, 720 * 2);
  const ctx = canvas.getContext("2d");
  ctx.scale(2, 2);
  await qr.toCanvas(canvas, config, {
    width: 720 * 2,
    color: { dark: "#023020" },
  });
  ctx.font = "16px Roboto Mono";
  ctx.fillStyle = "#023020";
  const nameWidth = ctx.measureText(Deno.args[4]).width;
  const allowedIPsWidth = ctx.measureText(Deno.args[1]).width;
  ctx.fillText(Deno.args[4], Math.round(360 - nameWidth / 2), 16);
  ctx.fillText(Deno.args[1], Math.round(360 - allowedIPsWidth / 2), 716);
  Deno.writeFileSync(
    Deno.args[4] + ".png",
    canvas.toBuffer("image/png"),
    {}
  );
}
