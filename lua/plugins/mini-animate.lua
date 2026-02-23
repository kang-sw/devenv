-- lua/plugins/mini-animate.lua
return {
  "nvim-mini/mini.animate",
  opts = function()
    local animate = require("mini.animate")
    return {
      scroll = {
        timing = animate.gen_timing.linear({ duration = 80, unit = "total" }),
      },
      cursor = {
        timing = animate.gen_timing.linear({ duration = 50, unit = "total" }),
      },
    }
  end,
}
