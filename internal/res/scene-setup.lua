local O = obslua

-- Settings
local instance_count = nil

local wall = nil
local wall_width = nil
local wall_height = nil
local freezing = nil

local locks = nil
local locks_path = nil
local locks_width = nil
local locks_height = nil

-- Script boilerplate, settings
function script_update(settings)
    -- Update the current settings.
    instance_count = O.obs_data_get_int(settings, "instance_count")
    wall = O.obs_data_get_bool(settings, "wall")
    wall_width = O.obs_data_get_int(settings, "wall_width")
    wall_height = O.obs_data_get_int(settings, "wall_height")
    freezing = O.obs_data_get_bool(settings, "wall_freezing")

    locks = O.obs_data_get_bool(settings, "locks")
    locks_path = O.obs_data_get_string(settings, "locks_path")
    locks_width = O.obs_data_get_int(settings, "locks_width")
    locks_height = O.obs_data_get_int(settings, "locks_height")
end

function script_description()
    return [[
        <center><h2>resetti scene setup</h2></center>
    ]]
end

function script_properties()
    local settings = O.obs_properties_create()
    local verif = O.obs_properties_create()
    local wall = O.obs_properties_create()

    -- General
    O.obs_properties_add_button(settings, "generate", "Generate", generate_scenes)
    O.obs_properties_add_int(settings, "instance_count", "Instance Count", 1, 32, 1)
    O.obs_properties_add_group(settings, "wall", "Wall", O.OBS_GROUP_CHECKABLE, wall)

    -- Wall
    O.obs_properties_add_int_slider(wall, "wall_width", "Width", 1, 12, 1)
    O.obs_properties_add_int_slider(wall, "wall_height", "Height", 1, 12, 1)
    O.obs_properties_add_bool(wall, "wall_freezing", "Freezing")

    local locks = O.obs_properties_create()
    O.obs_properties_add_path(locks, "locks_path", "File", O.OBS_PATH_FILE, "*.png *.jpg *.gif", nil)
    O.obs_properties_add_int(locks, "locks_width", "Width", 1, 3840, 1)
    O.obs_properties_add_int(locks, "locks_height", "Height", 1, 2160, 1)

    O.obs_properties_add_group(wall, "locks", "Lock Icons", O.OBS_GROUP_CHECKABLE, locks)

    return settings
end

-- Scene generation
function create_instance_scene(sources)
    -- Create scene
    local scene = O.obs_scene_create("Instance")
    local video_info = O.obs_video_info()
    O.obs_get_video_info(video_info)

    -- Create instance sources
    for i = 1, instance_count do
        local source = O.obs_source_create(
            "xcomposite_input",
            "MC " .. tostring(i),
            nil,
            nil
        )
        local item = O.obs_scene_add(scene, source)
        O.obs_sceneitem_set_bounds_type(item, O.OBS_BOUNDS_STRETCH)
        O.obs_sceneitem_set_scale_filter(item, O.OBS_SCALE_POINT)
        O.obs_sceneitem_set_locked(item, true)
        O.obs_sceneitem_set_visible(item, false)
        local vec2 = O.vec2()
        vec2.x = video_info.base_width
        vec2.y = video_info.base_height
        O.obs_sceneitem_set_bounds(item, vec2)
        vec2.x = 0
        vec2.y = 0
        O.obs_sceneitem_set_pos(item, vec2)

        table.insert(sources, source)
    end

    O.obs_scene_release(scene)
end

function create_wall_scene()
    -- Create scene
    local scene = O.obs_scene_create("Wall")
    local video_info = O.obs_video_info()
    O.obs_get_video_info(video_info)

    local inst_width = video_info.base_width / wall_width
    local inst_height = video_info.base_height / wall_height

    -- Create instance sources
    for i = 1, instance_count do
        local source = O.obs_source_create(
            "xcomposite_input",
            "Wall MC " .. tostring(i),
            nil,
            nil
        )
        local item = O.obs_scene_add(scene, source)
        O.obs_sceneitem_set_bounds_type(item, O.OBS_BOUNDS_STRETCH)
        O.obs_sceneitem_set_scale_filter(item, O.OBS_SCALE_POINT)
        O.obs_sceneitem_set_locked(item, true)
        O.obs_sceneitem_set_visible(item, true)
        local vec2 = O.vec2()
        vec2.x = inst_width
        vec2.y = inst_height
        O.obs_sceneitem_set_bounds(item, vec2)
        vec2.x = inst_width * ((i-1) % wall_width)
        vec2.y = inst_height * math.floor((i-1) / wall_width)
        O.obs_sceneitem_set_pos(item, vec2)

        if freezing then
            local data = O.obs_data_create_from_json([[
            {
                "hide_action": 2,
                "show_action": 1
            }
            ]])
            local freeze = O.obs_source_create("freeze_filter", "Freeze " .. tostring(i), data, nil)
            O.obs_source_filter_add(source, freeze)
            O.obs_source_release(freeze)
            O.obs_data_release(data)
        end
    end

    -- Create lock sources
    if locks_path then
        for i = 1, instance_count do
            local data = O.obs_data_create_from_json('{"file": "' .. locks_path .. '"}')
            local lock = O.obs_source_create(
                "image_source",
                "Lock " .. tostring(i),
                data,
                nil
            )

            item = O.obs_scene_add(scene, lock)
            O.obs_sceneitem_set_locked(item, true)
            local vec2 = O.vec2()
            vec2.x = inst_width * ((i-1) % wall_width)
            vec2.y = inst_height * math.floor((i-1) / wall_width)
            O.obs_sceneitem_set_pos(item, vec2)
            vec2.x = locks_width
            vec2.y = locks_height
            O.obs_sceneitem_set_bounds(item, vec2)
            O.obs_sceneitem_set_bounds_type(item, O.OBS_BOUNDS_STRETCH)
            O.obs_data_release(data)
            O.obs_source_release(lock)
            O.obs_source_release(source)
        end
    end

    O.obs_scene_release(scene)
end

function create_verification_scene(sources)
    -- Create scene
    local scene = O.obs_scene_create("Verification")
    local video_info = O.obs_video_info()
    O.obs_get_video_info(video_info)

    local inst_width = video_info.base_width / wall_width
    local inst_height = video_info.base_height / wall_height

    -- Create instance sources
    for i = 1, instance_count do
        local item = O.obs_scene_add(scene, sources[i])
        O.obs_sceneitem_set_bounds_type(item, O.OBS_BOUNDS_STRETCH)
        O.obs_sceneitem_set_scale_filter(item, O.OBS_SCALE_POINT)
        O.obs_sceneitem_set_locked(item, true)
        O.obs_sceneitem_set_visible(item, true)
        local vec2 = O.vec2()
        vec2.x = inst_width
        vec2.y = inst_height
        O.obs_sceneitem_set_bounds(item, vec2)
        vec2.x = inst_width * ((i-1) % wall_width)
        vec2.y = inst_height * math.floor((i-1) / wall_width)
        O.obs_sceneitem_set_pos(item, vec2)
    end

    O.obs_scene_release(scene)
end

function generate_scenes(_, _)
    -- Validate
    if wall and wall_height * wall_width < instance_count then
        error("Wall cannot fit instances.")
        return
    end

    print("Attempting to generate scenes.")

    local sources = {}

    -- Create the instance scene.
    create_instance_scene(sources)

    -- Create the wall scenes if necessary.
    if wall then
        create_wall_scene()
        create_verification_scene(sources)
    end

    for _, source in ipairs(sources) do
        O.obs_source_release(source)
    end
end
