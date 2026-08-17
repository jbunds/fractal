// TODO(jbunds): implement efficient anti-aliasing
//
// i tried implementing both 4x SSAA and FXAA, but the results were inadequate, and the adverse performance impact was unacceptable
//
// TODO(jbunds): investigate applying the two tricks mentioned at https://www.youtube.com/watch?v=uc2yok_pLV4&t=372s :
//
//   1. parallelize super sampling by with 4, 8, or 16 workers (his example illustrated at https://www.youtube.com/watch?v=uc2yok_pLV4&t=387s apparently uses 5 workers?)
//   2. render the next frame in the background while the current frame is displayed and transformed according to the scale (zoom) level (https://www.youtube.com/watch?v=uc2yok_pLV4&t=413s )

struct uniforms { // field declaration order must cohere with the corresponding Go struct field declaration order
  paletteSize:  u32,
  powScale:     u32,
  frameCounter: f32,
  iterations:   f32,

  width:        f32,
  height:       f32,
  scaleHi:      f32,
  scaleLo:      f32,

  xRealHi:      f32,
  yImagHi:      f32,
  xRealLo:      f32,
  yImagLo:      f32,

  cRealHi:      f32,
  cImagHi:      f32,
  cRealLo:      f32,
  cImagLo:      f32,
};

// TODO(jbunds): consider using a 1D texture to store the color palette and sample it, since it may
//               be faster than using a storage buffer due to GPU texture caching optimizations:
//
//   @group(0) @binding(1) var paletteTexture: texture_1d<f32>;
//   @group(0) @binding(2) var paletteSampler: sampler;
//
//   let color = textureSampleLevel(paletteTexture, paletteSampler, vec2<f32>(smoothIter * scale, 0.5), 0.0).rgb;

@group(0) @binding(0) var<uniform>       unis      : uniforms;
@group(0) @binding(1) var<storage, read> palette   : array<u32>;
@group(1) @binding(0) var                screenTex : texture_storage_2d<bgra8unorm, write>;

// emulated 64-bit floating-point arithmetic functions

fn to_dd(val: f32) -> vec2<f32> { return vec2<f32>(val, 0.0); }

fn dd_add(a: vec2<f32>, b: vec2<f32>) -> vec2<f32> {
  let s_x  =  a.x + b.x;
  let s_y  = (a.x - (s_x  - (s_x  - a.x))) + (b.x - (s_x - a.x));
  let t_x  =  a.y + b.y;
  let t_y  = (a.y - (t_x  - (t_x  - a.y))) + (b.y - (t_x - a.y));
  let c1   =  s_y + t_x;
  let v1_x =  s_x + c1;
  let v1_y = (s_x - (v1_x - (v1_x - s_x))) + (c1 - (v1_x - s_x));
  let rem  = v1_x + (v1_y + t_y);
  return vec2<f32>(rem, (v1_y + t_y) - (rem - v1_x));
}

fn dd_mul(a: vec2<f32>, b: vec2<f32>) -> vec2<f32> {
  let p_hi     = a.x * b.x;
  let p_lo     = fma(a.x, b.x, -p_hi);
  let cross    = a.x * b.y + a.y * b.x;
  let trailing = fma(a.y, b.y, cross + p_lo);
  let s        = p_hi + trailing;
  return vec2<f32>(s, trailing - (s - p_hi));
}

//fn dd_mag(x: vec2<f32>, y: vec2<f32>) -> f32 { // double-double |z|^2 = x^2 + y^2 computation
//  let x2  = dd_mul(x,  x );
//  let y2  = dd_mul(y,  y );
//  let mag = dd_add(x2, y2);
//  return mag.x + mag.y;
//}

fn smoothIter(i: u32, a: vec2<f32>, b: vec2<f32>) -> f32 {
  let mag2  = (a.x + b.x) + (a.y + b.y);
  let logZn = log2(mag2) * 0.5;
  let nu    = log2(logZn);
  return f32(i) + 1.0 - nu;
}
