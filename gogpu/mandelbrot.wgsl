fn mandelbrot(cx: vec2<f32>, cy: vec2<f32>) -> f32 {
  var x  = vec2<f32>(0.0);
  var y  = vec2<f32>(0.0);
  var x2 = vec2<f32>(0.0);
  var y2 = vec2<f32>(0.0);

  let limit = u32(unis.maxIter);
  var i     = 0u;

  for (; i < limit; i++) {
    y  = dd_add(dd_mul(x,   y ) * 2.0, cy);
    x  = dd_add(dd_add(x2, -y2),       cx);

    x2 = dd_mul(x, x);
    y2 = dd_mul(y, y);

    if (x2.x + y2.x > 4.0) { break; }
  }

  if (i >= limit) { return unis.maxIter; }

  return smoothIter(i, x2, y2);
}

@compute @workgroup_size(16, 8, 1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
  if (id.x >= u32(unis.width) || id.y >= u32(unis.height)) { return; }

  let screenX = f32(id.x);
  let screenY = f32(id.y);
  let minDim  = min(unis.width, unis.height);

  let scale   = vec2<f32>(unis.scaleHi, unis.scaleLo);
  let targetX = vec2<f32>(unis.xRealHi, unis.xRealLo);
  let targetY = vec2<f32>(unis.yImagHi, unis.yImagLo);

  let scaleX  = to_dd((screenX - (unis.width / 2.0)) / minDim);
  let scaleY  = to_dd((unis.height / 2.0 - screenY)  / minDim); // y-axis inverted; maps WebGPU coordinates to cartesian coordinates

  let cx      = dd_add(targetX, dd_mul(scaleX, scale));
  let cy      = dd_add(targetY, dd_mul(scaleY, scale));

  let iter    = mandelbrot(cx, cy);

  var color: vec4<f32>;
  if (iter >= unis.maxIter) {
    color = vec4<f32>(0.0, 0.0, 0.0, 1.0);
  } else {
    let dyn_div     = 80.0 + (unis.frameCount * 0.4);
    let clamped_div = min(dyn_div, unis.maxIter);
    let idx         = u32((iter / clamped_div) * f32(unis.paletteSize)) % unis.paletteSize;
    color           = unpack4x8unorm(textureLoad(palette, i32(idx)).x);
  }

  textureStore(screenTex, vec2<u32>(id.xy), color);
}
