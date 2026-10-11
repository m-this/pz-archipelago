function Dump_table(o, depth)
    if depth == nil then
        depth = 0
    end

    local ignore_keys = {
        type_chart = true,
    }

    if type(o) == 'table' then
        local tabs = ('\t'):rep(depth)
        local tabs2 = ('\t'):rep(depth + 1)
        local s = '{\n'
        for k, v in pairs(o) do
            local key_str = tostring(k)
            if not ignore_keys[key_str] then
                if type(k) ~= 'number' then
                    k = '"' .. k .. '"'
                end
                s = s .. tabs2 .. '[' .. k .. '] = ' .. Dump_table(v, depth + 1) .. ',\n'
            end
        end
        return s .. tabs .. '}'
    else
        return tostring(o)
    end
end

-- Dumps slot data later

function Has(item, amount)
    local count = Tracker:ProviderCountForCode(item)
    local amount = tonumber(amount)
    if not amount then
        if count > 0 then
            return AccessibilityLevel.Normal
        end
    elseif count >= amount then
        return AccessibilityLevel.Normal
    end
    return AccessibilityLevel.None
end

-- So Has function works with an 'or' case
function Has_new(item, amount)
    local count = Tracker:ProviderCountForCode(item)
    amount = tonumber(amount)
    if not amount then
        return count > 0
    else
        return count >= amount
    end
end

function Has_location(loc)
    if loc.AvailableChestCount == 0 then
        return AccessibilityLevel.Normal
    else
        return AccessibilityLevel.None
    end
end

function ProgCount(code)
    return Tracker:FindObjectForCode(code).CurrentStage
end

--returns whether we can access all given arguments
function Access(...)
    local access = AccessibilityLevel.Normal
    local args = { ... }
    for i, v in ipairs(args) do
        --break early if we hit min accessibility
        if v == AccessibilityLevel.None then
            return AccessibilityLevel.None
        end

        if access > v then
            access = v
        end
    end

    return access
end

--returns the maximum accessibility of the given arguments
function Max(...)
    local maximum = AccessibilityLevel.None
    local args = { ... }

    for i, v in ipairs(args) do
        if v == AccessibilityLevel.Normal then
            return AccessibilityLevel.Normal
        end

        if maximum < v then
            maximum = v
        end
    end
    return maximum
end

function Scoutable()
    return AccessibilityLevel.Inspect
end

function Get_ap_locations()
    -- We get all locations from AP.
    -- They only share checked & missing ones,
    -- so we combine them ourselves.
    local missing = Archipelago.MissingLocations
    local locations = Archipelago.CheckedLocations
    local existing_locations = {}

    --loop through all checked and unchecked locations and combine them
    for _, v in pairs(missing) do
        existing_locations[v] = true
    end
    for _, v in pairs(locations) do
        existing_locations[v] = true
    end

    return existing_locations
end
