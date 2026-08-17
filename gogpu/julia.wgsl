fn julia(cx: vec2<f32>, cy: vec2<f32>) -> f32 {
  let c_real = vec2<f32>(p.cRealHi, p.cRealLo);
  let c_imag = vec2<f32>(p.cImagHi, p.cImagLo);

  var x  = cx;
  var y  = cy;
  var x2 = dd_mul(x, x);
  var y2 = dd_mul(y, y);

  let limit = u32(p.iterations);
  var i     = 0u;

  for (; i < limit; i++) {
    y  = dd_add(dd_mul(x,   y ) * 2.0, c_imag);
    x  = dd_add(dd_add(x2, -y2),       c_real);

    x2 = dd_mul(x, x);
    y2 = dd_mul(y, y);

    if (x2.x + y2.x > 4.0) { break; }
  }

  if (i >= limit) { return p.iterations; }

  let mag2_hi = x2.x    + y2.x;
  let mag2_lo = x2.y    + y2.y;
  let mag2    = mag2_hi + mag2_lo;

// proven good:
//  return f32(i) + 1.0 - log2(log2(mag2) * 0.5);

// non-smoothIter version:
//  return f32(i) + 1.0 - log2(log2(x2.x + y2.x) * 0.5);

// smoothIter version:
  let logZn = log2(mag2) / 2.0;
  let nu    = log2(logZn / log2(2.0)) / log2(2.0);
  return f32(i) + 1.0 - nu;
}

@compute @workgroup_size(16, 8, 1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
  if (id.x >= u32(p.width) || id.y >= u32(p.height)) { return; }

  let screenX = f32(id.x);
  let screenY = f32(id.y);
  let minDim  = min(p.width, p.height);

  let zoom    = vec2<f32>(p.zoomHi,    p.zoomLo);
  let targetX = vec2<f32>(p.targetXHi, p.targetXLo);
  let targetY = vec2<f32>(p.targetYHi, p.targetYLo);

  let scaleX  = to_dd((screenX - (p.width / 2.0)) / minDim);
// TODO(jbunds): determine why the y-axis is inverted in the Mandelbrot set but not here
//  let scaleY  = to_dd((p.height / 2.0 - screenY)  / minDim);
  let scaleY  = to_dd((screenY - (p.height / 2.0)) / minDim);

  let cx      = dd_add(targetX, dd_mul(scaleX, zoom));
  let cy      = dd_add(targetY, dd_mul(scaleY, zoom));

  let iter    = julia(cx, cy);

  var color: vec4<f32>;
  if (iter >= p.iterations) {
    color = vec4<f32>(0.0, 0.0, 0.0, 1.0);
  } else {

    var normal_t: f32;

    if (p.usePowScale == 1u) {
      normal_t = pow(iter / p.iterations, 0.6);
    } else {
      normal_t =     iter / p.iterations;
    }

    let idx = u32(normal_t * f32(p.paletteSize)) % p.paletteSize;
    color   = unpack4x8unorm(palette[idx]);
  }

  textureStore(screenTex, vec2<u32>(id.xy), color);
}
