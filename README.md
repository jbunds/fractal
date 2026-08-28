[gogpu]:  https://github.com/gogpu/gogpu
[wgsl]:   https://en.wikipedia.org/wiki/WebGPU_Shading_Language
[smooth]: https://github.com/jbunds/fractal/blob/0952d1fc97c15ef0f1f8ce775194c7023719e7ad/resources.go#L77
[mandel]: https://github.com/jbunds/fractal/blob/4dde21b10bf4e699d64915b5f70ee18d88b0603f/mandelbrot.wgsl#L49
[julia]:  https://github.com/jbunds/fractal/blob/4dde21b10bf4e699d64915b5f70ee18d88b0603f/julia.wgsl#L54

### Yet Another Fractal Renderer

This program uses [`github.com/gogpu/gogpu`][gogpu] with some basic [WGSL][wgsl] shader code to render and zoom in on Mandelbrot and filled Julia set fractals.

The quality of the rendered fractals, in terms of the fine structure details revealed, and the depth and steepness of the color gradients applied to those structures, is compromised to varying degrees (depending on the specific fractal) by the choice of the set of _one-size-fits-all_ parameter values in certain functions:

- the [constants][smooth] in the cosine interpolation function used to smooth color transitions in the pre-computed color palette
- the constants in the functions used by the shaders to select per-pixel colors from the pre-computed color palette:
  - [`mandelbrot.wsgl`][mandel]
  - [`julia.wgsl`][julia]

This was a deliberate design decision motivated by the desire to keep the core math functions relatively simple, at least for the first working versions of the program. The quality of the rendered fractals can definitely be improved by using more sophisticated math functions chosen specifically for each individual fractal, or perhaps subsets of fractals.

No anti-aliasing is applied to the rendered images, further degrading their quality, especially as each frame transitions to the next increased-magnification rendering (i.e., high-gradient regions appear to scintillate or flicker as the viewport zooms in).

### Usage

```
$ go run . -help
fractal usage:

  -fractal string
        canonical fractal name ("seahorse", "dendrite", etc) (default "elephant")
  -theme string
        color scheme ("green" or "red") (default "green")
  -type string
        fractal type ("mandelbrot" or "julia") (default "mandelbrot")
```
