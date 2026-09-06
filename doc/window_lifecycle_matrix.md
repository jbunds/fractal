# Window Lifecycle & Event Permutation Matrix

This document provides an exhaustive matrix of all combinations of GUI events and window-lifecycle states in the application. It serves as an architectural reference and guides the window-lifecycle testing strategy.

---

## 1. State Dimensions

The application's runtime lifecycle is fully determined by a 3-tuple:
`(PrimaryVisible, AboutVisible, AnimationIntent)`

| Dimension           | Domain              | Description                                                             |
| :------------------ | :------------------ | :---------------------------------------------------------------------- |
| **Primary**         | `{visible, hidden}` | main application window / fractal rendering window                      |
| **About**           | `{visible, hidden}` | translucent About dialog window                                         |
| **AnimationIntent** | `{playing, paused}` | persistent user intent (`animating atomic.Bool`), toggled via space bar |

This produces **8 distinct reachable states** ($2 \times 2 \times 2$):

| State ID | Primary | About   | Animation Intent | Description                                              |
| :------- | :------ | :------ | :--------------- | :------------------------------------------------------- |
| **S1**   | visible | hidden  | playing          | default startup state                                    |
| **S2**   | visible | hidden  | paused           | primary window visible, paused via pressing space bar    |
| **S3**   | visible | visible | playing          | both windows open, actively animating                    |
| **S4**   | visible | visible | paused           | both windows open, animation paused                      |
| **S5**   | hidden  | visible | playing          | primary closed while About remained open (intent: play)  |
| **S6**   | hidden  | visible | paused           | primary closed while About remained open (intent: pause) |
| **S7**   | hidden  | hidden  | playing          | both windows closed (intent: play)                       |
| **S8**   | hidden  | hidden  | paused           | both windows closed (intent: pause)                      |

---

## 2. Supported GUI Actions / Events

| Action                  | Event                                                              |
| :---------------------- | :----------------------------------------------------------------- |
| **`Act_CmdW`**          | press ⌘+W keyboard shortcut                                        |
| **`Act_ClosePrimary`**  | click native "red circle" close button in primary window title bar |
| **`Act_CloseAbout`**    | click native "red circle" close button in About window title bar   |
| **`Act_OpenAbout`**     | select **About Fractal** from application menu                     |
| **`Act_SelectFractal`** | select any fractal from **Fractals** menu                          |
| **`Act_SelectTheme`**   | select any theme from **Themes** menu                              |
| **`Act_SpaceBar`**      | press space bar (toggles animation play / pause)                   |

---

## 3. Core Invariants & Rules

1. **Window Hierarchy (⌘+W)**:
   - When **both** windows are visible (**S3**, **S4**), **⌘+W must hide whichever window currently has focus**.
   - Only when the primary window has focus does ⌘+W close the primary window.
   - Only when the About window has focus does ⌘+W close the About window.
2. **Close Button Disambiguation**:
   - Clicking the "red circle" button on the About window closes ONLY the About window.
   - Clicking the "red circle" button on the Primary window closes ONLY the primary window.
3. **Persistent Animation State**:
   - Hiding the primary window stops the underlying GPU animation loop (`gogpu.AnimationToken`) to prevent burning CPU and GPU cycles on invisible windows.
   - The user intent (`animating`) is strictly preserved across window `Show()` / `Hide()` cycles.
   - Reopening the primary window (or selecting a fractal or theme from the menu) resumes animation if intent is `playing`, and stays paused if intent is `paused`.
4. **Static Redraws While Paused**:
   - Selecting a new fractal or theme while paused issues a single redraw request (`RequestRedraw()`) to update the display.
   - `renderer.draw()` advances `viewportWidth` and `frameCount` **only when animating**. Redraws while paused re-render the current frame in the new theme / fractal without advancing zoom magnification.
5. **Mutex Deadlock Prevention**:
   - Calling `*gogpu.Window.Hide()` directly within `SetOnClose` or `SetOnKeyPress` causes a non-reentrant mutex deadlock in GoGPU (`win.mu.Lock()`).
   - Window hiding is always deferred from the respective window's `SetOnClose` or `SetOnKeyPress` handler to `app.OnUpdate()` via atomic flags (`hidePrimaryWindow`, `hideAboutWindow`).

---

## 4. The 56-Permutation State Transition Matrix

| Current State                | Event              | Target / Result                       | Next State             | Animation Loop | Redraw Requested? |
| :--------------------------- | :----------------- | :------------------------------------ | :--------------------- | :------------- | :---------------- |
| **S1** (P_vis, A_hid, Play)  | **⌘+W**            | primary hides                         | **S7** (~P, ~A, Play)  | stops          | no                |
|                              | **Close Primary**  | primary hides                         | **S7** (~P, ~A, Play)  | stops          | no                |
|                              | **Close About**    | no-op (already hidden)                | **S1**                 | running        | no                |
|                              | **Open About**     | About shows                           | **S3** ( P,  A, Play)  | running        | no                |
|                              | **Select Fractal** | reloads fractal (or no-op)            | **S1** ( P, ~A, Play)  | running        | loop (VSync)      |
|                              | **Select Theme**   | reloads theme (or no-op)              | **S1** ( P, ~A, Play)  | running        | loop (VSync)      |
|                              | **Space Bar**      | pauses animation                      | **S2** ( P, ~A, Pause) | stops          | no                |
| **S2** (P_vis, A_hid, Pause) | **⌘+W**            | primary hides                         | **S8** (~P, ~A, Pause) | stopped        | no                |
|                              | **Close Primary**  | primary hides                         | **S8** (~P, ~A, Pause) | stopped        | no                |
|                              | **Close About**    | no-op (already hidden)                | **S2**                 | stopped        | no                |
|                              | **Open About**     | About shows                           | **S4** ( P,  A, Pause) | stopped        | no                |
|                              | **Select Fractal** | reloads fractal (or no-op)            | **S2** ( P, ~A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Select Theme**   | reloads theme (or no-op)              | **S2** ( P, ~A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Space Bar**      | resumes animation                     | **S1** ( P, ~A, Play)  | starts         | loop (VSync)      |
| **S3** (P_vis, A_vis, Play)  | **⌘+W**            | **About hides** (P remains visible)   | **S1** ( P, ~A, Play)  | running        | no                |
|                              | **Close Primary**  | primary hides                         | **S5** (~P,  A, Play)  | stops          | no                |
|                              | **Close About**    | About hides                           | **S1** ( P, ~A, Play)  | running        | no                |
|                              | **Open About**     | focuses About                         | **S3** ( P,  A, Play)  | running        | no                |
|                              | **Select Fractal** | reloads fractal (or no-op; P visible) | **S3** ( P,  A, Play)  | running        | loop (VSync)      |
|                              | **Select Theme**   | reloads theme (or no-op; P visible)   | **S3** ( P,  A, Play)  | running        | loop (VSync)      |
|                              | **Space Bar**      | pauses animation                      | **S4** ( P,  A, Pause) | stops          | no                |
| **S4** (P_vis, A_vis, Pause) | **⌘+W**            | **About hides** (P remains visible)   | **S2** ( P, ~A, Pause) | stopped        | no                |
|                              | **Close Primary**  | primary hides                         | **S6** (~P,  A, Pause) | stopped        | no                |
|                              | **Close About**    | About hides                           | **S2** ( P, ~A, Pause) | stopped        | no                |
|                              | **Open About**     | focuses About                         | **S4** ( P,  A, Pause) | stopped        | no                |
|                              | **Select Fractal** | reloads fractal (or no-op)            | **S4** ( P,  A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Select Theme**   | reloads theme (or no-op)              | **S4** ( P,  A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Space Bar**      | resumes animation                     | **S3** ( P,  A, Play)  | starts         | loop (VSync)      |
| **S5** (~P, A_vis, Play)     | **⌘+W**            | About hides                           | **S7** (~P, ~A, Play)  | stopped        | no                |
|                              | **Close Primary**  | no-op (already hidden)                | **S5**                 | stopped        | no                |
|                              | **Close About**    | About hides                           | **S7** (~P, ~A, Play)  | stopped        | no                |
|                              | **Open About**     | focuses About                         | **S5** (~P,  A, Play)  | stopped        | no                |
|                              | **Select Fractal** | primary shows                         | **S3** ( P,  A, Play)  | resumes        | loop (VSync)      |
|                              | **Select Theme**   | primary shows                         | **S3** ( P,  A, Play)  | resumes        | loop (VSync)      |
|                              | **Space Bar**      | flips intent to Pause                 | **S6** (~P,  A, Pause) | stopped        | no                |
| **S6** (~P, A_vis, Pause)    | **⌘+W**            | About hides                           | **S8** (~P, ~A, Pause) | stopped        | no                |
|                              | **Close Primary**  | no-op (already hidden)                | **S6**                 | stopped        | no                |
|                              | **Close About**    | About hides                           | **S8** (~P, ~A, Pause) | stopped        | no                |
|                              | **Open About**     | focuses About                         | **S6** (~P,  A, Pause) | stopped        | no                |
|                              | **Select Fractal** | primary shows                         | **S4** ( P,  A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Select Theme**   | primary shows                         | **S4** ( P,  A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Space Bar**      | flips intent to Play                  | **S5** (~P,  A, Play)  | stopped        | no                |
| **S7** (~P, ~A, Play)        | **⌘+W**            | no-op (all hidden)                    | **S7**                 | stopped        | no                |
|                              | **Close P / A**    | no-op (all hidden)                    | **S7**                 | stopped        | no                |
|                              | **Open About**     | About shows                           | **S5** (~P,  A, Play)  | stopped        | no                |
|                              | **Select Fractal** | primary shows                         | **S1** ( P, ~A, Play)  | resumes        | loop (VSync)      |
|                              | **Select Theme**   | primary shows                         | **S1** ( P, ~A, Play)  | resumes        | loop (VSync)      |
|                              | **Space Bar**      | flips intent to Pause                 | **S8** (~P, ~A, Pause) | stopped        | no                |
| **S8** (~P, ~A, Pause)       | **⌘+W**            | no-op (all hidden)                    | **S8**                 | stopped        | no                |
|                              | **Close P / A**    | no-op (all hidden)                    | **S8**                 | stopped        | no                |
|                              | **Open About**     | About shows                           | **S6** (~P,  A, Pause) | stopped        | no                |
|                              | **Select Fractal** | primary shows                         | **S2** ( P, ~A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Select Theme**   | primary shows                         | **S2** ( P, ~A, Pause) | stopped        | **yes (1 frame)** |
|                              | **Space Bar**      | flips intent to Play                  | **S7** (~P, ~A, Play)  | stopped        | no                |
