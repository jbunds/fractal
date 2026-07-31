// something something something ... with software-emulated 64-bit floating-point math
struct Params {   // 48 bytes total
  width:     f32, // bytes  0 -  3 (block 1)
  height:    f32, // bytes  4 -  7
  maxIter:   f32, // bytes  8 - 11
	subStep:   f32, // bytes 12 - 15

  zoomHi:    f32, // bytes 16 - 19 (block 2)
	zoomLo:    f32, // bytes 20 - 23
  targetXHi: f32, // bytes 24 - 27
  targetYHi: f32, // bytes 28 - 31

	targetXLo: f32, // bytes 32 - 35 (block 3)
	targetYLo: f32, // bytes 36 - 39
	pad0:      f32, // bytes 40 - 43
	pad1:      f32, // bytes 44 - 47
};

@group(0) @binding(0) var<uniform> p: Params;
@group(0) @binding(1) var screenTex: texture_storage_2d<bgra8unorm, write>;
//@group(0) @binding(2) var<storage, read> palette: array<vec4<f32>>; // 2000-color palette buffer

// emulated 64-bit floating-point math functions

// emulates addition of two double-single values
fn ds_add(a: vec2<f32>, b: vec2<f32>) -> vec2<f32> {
  let s = a.x + b.x;
	let v = s - a.x;
	let e = (a.x - (s - v)) + (b.x - v) + a.y + b.y;
	return vec2<f32>(s + e, e - ((s + e) - s));
}

// emulates multiplication of two double-single values
fn ds_mul(a: vec2<f32>, b: vec2<f32>) -> vec2<f32> {
  let c = a.x * b.x;
	let e = fma(a.x, b.x, -c) + a.x * b.y + a.y * b.x;
	return vec2<f32>(c + e, e - ((c + e) - c));
}

// converts a single standard f32 to an emulated fp64 vec2
fn to_ds(val: f32) -> vec2<f32> {
  return vec2<f32>(val, 0.0);
}

fn mandelbrot(cx: vec2<f32>, cy: vec2<f32>) -> f32 {
  var i  = 0.0;
  var x  = vec2<f32>(0.0, 0.0);
  var y  = vec2<f32>(0.0, 0.0);
  var x2 = vec2<f32>(0.0, 0.0);
  var y2 = vec2<f32>(0.0, 0.0);

  for (i = 0.0; i < p.maxIter; i += 1.0) {
    // nextY = x * y * 2.0 + cy
    let xy    = ds_mul(x, y);
    let nextY = ds_add(ds_add(xy, xy), cy);
    
    // x = x2 - y2 + cx
    let neg_y2 = -y2;
    x          = ds_add(ds_add(x2, neg_y2), cx);
    y          = nextY;
    
    // cache squares for escape test and subsequent loop step
    x2 = ds_mul(x, x);
    y2 = ds_mul(y, y);
    
    // escape condition test using high bits for performance
    if (x2.x + y2.x > 4.0) { break; }
  }

  if (i >= p.maxIter) { return p.maxIter; }
  
  // smooth coloring math can safely utilize regular f32 resolution
//  let log2Zn = log2(max(x2.x + y2.x, 0.0001)) / 2.0;
//  let nu     = log2(max(log2Zn, 0.0001));
  let log2Zn = log2(x2.x + y2.x) / 2.0;
  let nu     = log2(log2Zn);
  return i + 1.0 - nu;
}

// no AA
@compute @workgroup_size(16, 16, 1)
fn main(@builtin(global_invocation_id) id: vec3<u32>) {
  if (id.x >= u32(p.width) || id.y >= u32(p.height)) { return; }

  let screenX = f32(id.x);
  let screenY = f32(p.height) - f32(id.y);
//  let screenY = f32(id.y);
  let minDim  = min(p.width, p.height);

  // reconstruct emulated parameters from high/low pairs
  let zoom    = vec2<f32>(p.zoomHi,    p.zoomLo);
  let targetX = vec2<f32>(p.targetXHi, p.targetXLo);
  let targetY = vec2<f32>(p.targetYHi, p.targetYLo);

  // normalize pixel locations: screenScale = (screen - half) / minDim
  let scaleX  = to_ds((screenX - (p.width  / 2.0)) / minDim);
  let scaleY  = to_ds((screenY - (p.height / 2.0)) / minDim);

  // cx = targetX + scaleX * zoom
  let cx = ds_add(targetX, ds_mul(scaleX, zoom));
  let cy = ds_add(targetY, ds_mul(scaleY, zoom));

  let iter = mandelbrot(cx, cy);

  var color: vec4<f32>;
  if (iter >= p.maxIter) {
    color = vec4<f32>(0.0, 0.0, 0.0, 1.0); // interior regions are rendered in black
  } else {
//    let log_iter = log(iter + 1.0);
    let r = sin(0.015 * iter + 1.0) * 0.5 + 0.5;
    let g = sin(0.012 * iter + 2.0) * 0.5 + 0.5;
    let b = sin(0.010 * iter + 4.0) * 0.5 + 0.5;
//    let r = sin(2.5 * log_iter + 0.0) * 0.5 + 0.5;
//    let g = sin(2.0 * log_iter + 2.0) * 0.5 + 0.5;
//    let b = sin(1.5 * log_iter + 4.0) * 0.5 + 0.5;
//    let r = sin(0.3 * log_iter + 0.0) * 0.5 + 0.5;
//    let g = sin(0.6 * log_iter + 2.0) * 0.5 + 0.5;
//    let b = sin(0.9 * log_iter + 4.0) * 0.5 + 0.5;
    color = vec4<f32>(r, g, b, 1.0);

//    // quilez
//    let t      = iter / p.maxIter;           // smooth normalization of the iteration space between 0.0 and 1.0
//    let norm_t = pow(t, 0.5);                // apply a gentle geometric power curve to stretch the gradient near filaments to expand dark bands and smoothen transitions
//    let pi_2   = 6.28318530718;              // Iñigo Quilez cosine palette algorithm: color = a + b * cos(2 * Pi * (c * t + d))
//    let a      = vec3<f32>(0.5, 0.5,  0.5 ); // brightness
//    let b      = vec3<f32>(0.5, 0.5,  0.5 ); // contrast
//    let c      = vec3<f32>(1.0, 1.0,  1.0 ); // frequency
//    let d      = vec3<f32>(0.0, 0.33, 0.67); // phase shift (r, g, b offsets)
//    let rgb    = a + b * cos(pi_2 * (c * norm_t + d));
//    color = vec4<f32>(rgb, 1.0);

  }

  textureStore(screenTex, vec2<i32>(id.xy), color);
}

// 4x SSAA
//@compute @workgroup_size(16, 16, 1)
//fn main(@builtin(global_invocation_id) id: vec3<u32>) {
//  if (id.x >= u32(p.width) || id.y >= u32(p.height)) { return; }
//
//  let screenX = f32(id.x);
//  let screenY = f32(p.height) - f32(id.y);
//  let minDim  = min(p.width, p.height);
//
//  let zoom    = vec2<f32>(p.zoomHi,    p.zoomLo);
//  let targetX = vec2<f32>(p.targetXHi, p.targetXLo);
//  let targetY = vec2<f32>(p.targetYHi, p.targetYLo);
//
//  // 4x ordered grid super sampling offsets (halfway into sub-pixel quadrants)
//  // sub-pixels sit at (-0.25, -0.25), (+0.25, -0.25), (-0.25, +0.25), (+0.25, +0.25)
//  var offsets = array<vec2<f32>, 4>(
//    vec2<f32>(-0.25, -0.25),
//    vec2<f32>( 0.25, -0.25),
//    vec2<f32>(-0.25,  0.25),
//    vec2<f32>( 0.25,  0.25)
//  );
//
//  var accumulated_color = vec3<f32>(0.0, 0.0, 0.0);
//
//  // loop through all 4 sub-pixel samples
//  for (var s = 0; s < 4; s += 1) {
//    let subX = screenX + offsets[s].x;
//    let subY = screenY + offsets[s].y;
//
//    let scaleX = to_ds((subX - (p.width  / 2.0)) / minDim);
//    let scaleY = to_ds((subY - (p.height / 2.0)) / minDim);
//
//    let cx = ds_add(targetX, ds_mul(scaleX, zoom));
//    let cy = ds_add(targetY, ds_mul(scaleY, zoom));
//
//    let iter = mandelbrot(cx, cy);
//
//    var sample_color: vec3<f32>;
//    if (iter >= p.maxIter) {
//      sample_color = vec3<f32>(0.0, 0.0, 0.0);
//    } else {
//      let r = sin(0.015 * iter + 1.0) * 0.5 + 0.5;
//      let g = sin(0.012 * iter + 2.0) * 0.5 + 0.5;
//      let b = sin(0.010 * iter + 4.0) * 0.5 + 0.5;
//      sample_color = vec3<f32>(r, g, b);
//    }
//    
//    accumulated_color += sample_color;
//  }
//
//  // average the 4 sub-pixel samples together to get the final anti-aliased color
//  let final_color = vec4<f32>(accumulated_color * 0.25, 1.0);
//
//  textureStore(screenTex, vec2<i32>(id.xy), final_color);
//}

// FXAA
//@compute @workgroup_size(16, 16, 1)
//fn main(@builtin(global_invocation_id) id: vec3<u32>) {
//  if (id.x >= u32(p.width) || id.y >= u32(p.height)) { return; }
//
//  let screenX = f32(id.x);
//  let screenY = f32(p.height) - f32(id.y);
//  let minDim  = min(p.width, p.height);
//
//  let zoom    = vec2<f32>(p.zoomHi,    p.zoomLo);
//  let targetX = vec2<f32>(p.targetXHi, p.targetXLo);
//  let targetY = vec2<f32>(p.targetYHi, p.targetYLo);
//
//  let scaleX = to_ds((screenX - (p.width  / 2.0)) / minDim);
//  let scaleY = to_ds((screenY - (p.height / 2.0)) / minDim);
//
//  let cx = ds_add(targetX, ds_mul(scaleX, zoom));
//  let cy = ds_add(targetY, ds_mul(scaleY, zoom));
//
//  let iter = mandelbrot(cx, cy);
//
//  var current_color: vec3<f32>;
//  if (iter >= p.maxIter) {
//    current_color = vec3<f32>(0.0, 0.0, 0.0);
//  } else {
//    let r = sin(0.015 * iter + 1.0) * 0.5 + 0.5;
//    let g = sin(0.012 * iter + 2.0) * 0.5 + 0.5;
//    let b = sin(0.010 * iter + 4.0) * 0.5 + 0.5;
//    current_color = vec3<f32>(r, g, b);
//  }
//
//  // neighborhood blending (low-latency anti-aliasing)
//  // check the mathematical "next pixel over" to calculate a spatial delta
//  let neighborScaleX = to_ds(((screenX + 1.0) - (p.width / 2.0)) / minDim);
//  let neighborCX     = ds_add(targetX, ds_mul(neighborScaleX, zoom));
//  let neighborIter   = mandelbrot(neighborCX, cy); // occasionally test only one extra ray
//
//  var neighbor_color: vec3<f32>;
//  if (neighborIter >= p.maxIter) {
//    neighbor_color = vec3<f32>(0.0, 0.0, 0.0);
//  } else {
//    let r = sin(0.015 * neighborIter + 1.0) * 0.5 + 0.5;
//    let g = sin(0.012 * neighborIter + 2.0) * 0.5 + 0.5;
//    let b = sin(0.010 * neighborIter + 4.0) * 0.5 + 0.5;
//    neighbor_color = vec3<f32>(r, g, b);
//  }
//
//  // if the color delta between a pixel and its direct neighbor is high, blend them by 15% to eliminate sub-pixel flashing across frames
//  let delta = distance(current_color, neighbor_color);
//  var final_color = current_color;
//  if (delta > 0.3) {
//    final_color = mix(current_color, neighbor_color, 0.15);
//  }
//
//  textureStore(screenTex, vec2<i32>(id.xy), vec4<f32>(final_color, 1.0));
//}
