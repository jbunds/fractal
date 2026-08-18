fn julia(cx: vec2<f32>, cy: vec2<f32>) -> f32 {
  let c_real = vec2<f32>(unis.cRealHi, unis.cRealLo);
  let c_imag = vec2<f32>(unis.cImagHi, unis.cImagLo);

  var x  = cx;
  var y  = cy;
  var x2 = dd_mul(x, x);
  var y2 = dd_mul(y, y);

  let limit = u32(unis.maxIter);
  var i     = 0u;

  for (; i < limit; i++) {
    y  = dd_add(dd_mul(x,   y ) * 2.0, c_imag);
    x  = dd_add(dd_add(x2, -y2),       c_real);

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

  let scaleX  = to_dd((screenX - (unis.width  / 2.0)) / minDim);
  let scaleY  = to_dd((screenY - (unis.height / 2.0)) / minDim); // y-axis not inverted; viewport center == cartesian center

  let cx      = dd_add(targetX, dd_mul(scaleX, scale));
  let cy      = dd_add(targetY, dd_mul(scaleY, scale));

  let iter    = julia(cx, cy);

  var color: vec4<f32>;
  if (iter >= unis.maxIter) {
    color = vec4<f32>(0.0, 0.0, 0.0, 1.0);
  } else {
    let t_linear = iter / unis.maxIter;
    let t_log    = log(iter + 1.0) / log(unis.maxIter + 1.0);
    let t_power  = pow(t_log, 1.8);
    let normal_t = select(t_linear, t_power, unis.powScale == 1u);
    let idx      = u32(normal_t * f32(unis.paletteSize)) % unis.paletteSize;
    color        = unpack4x8unorm(palette[idx]);
  }

  textureStore(screenTex, vec2<u32>(id.xy), color);
}
