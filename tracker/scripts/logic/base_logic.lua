-- this is the file to put all your custom logic functions into.
-- if you dont want to use the json based logic you can switch to a graph-based logic method.
-- the needed functions for that are in `/scripts/logic/graph_logic/logic_main.lua`.



-- function <name> (<parameters if needed>)
--     <actual code>
--     <indentations are just for readability>
-- end
--
-- (count_Permits moved below: no item carries a bare 'Permit' code, so the old ProviderCountForCode('Permit') always returned 0)

-- Every species permit code in items.json (generated 2026-09-18 from items/items.json; regenerate if permits change).
PERMIT_CODES = {
    "Permit_Aardvark",
    "Permit_Africanbuffalo",
    "Permit_Africanleopard",
    "Permit_Africansavannahelephant",
    "Permit_Africanwilddog",
    "Permit_Aldabragianttortoise",
    "Permit_Amazoniangiantcentipede",
    "Permit_Americanbison",
    "Permit_Bactriancamel",
    "Permit_Bairdstapir",
    "Permit_Bengaltiger",
    "Permit_Blackandwhiteruffedlemur",
    "Permit_Blackwildebeest",
    "Permit_Boaconstrictor",
    "Permit_Bongo",
    "Permit_Bonobo",
    "Permit_Borneanorangutan",
    "Permit_Braziliansalmonpinktarantula",
    "Permit_Brazilianwanderingspider",
    "Permit_Cheetah",
    "Permit_Chinesepangolin",
    "Permit_Collaredpeccary",
    "Permit_Commondeathadder",
    "Permit_Commonostrich",
    "Permit_Commonwarthog",
    "Permit_Easternbrownsnake",
    "Permit_Formosanblackbear",
    "Permit_Galapagosgianttortoise",
    "Permit_Gemsbok",
    "Permit_Gharial",
    "Permit_Giantburrowingcockroach",
    "Permit_Giantdeserthairyscorpion",
    "Permit_Giantforestscorpion",
    "Permit_Giantpanda",
    "Permit_Gianttigerlandsnail",
    "Permit_Gilamonster",
    "Permit_Goldenpoisonfrog",
    "Permit_Goliathbeetle",
    "Permit_Goliathbirdeater",
    "Permit_Goliathfrog",
    "Permit_Greaterflamingo",
    "Permit_Greeniguana",
    "Permit_Grizzlybear",
    "Permit_Himalayanbrownbear",
    "Permit_Hippopotamus",
    "Permit_Indianelephant",
    "Permit_Indianpeafowl",
    "Permit_Indianrhinoceros",
    "Permit_Japanesemacaque",
    "Permit_Lehmannpoisonfrog",
    "Permit_Lesserantilleaniguana",
    "Permit_Malabarrose",
    "Permit_Mandrill",
    "Permit_Mexicanredkneetarantula",
    "Permit_Nilemonitor",
    "Permit_Nyala",
    "Permit_Okapi",
    "Permit_Plainszebra",
    "Permit_Pronghornantelope",
    "Permit_Puffadder",
    "Permit_Reddeer",
    "Permit_Redpanda",
    "Permit_Redruffedlemur",
    "Permit_Reticulatedgiraffe",
    "Permit_Ringtailedlemur",
    "Permit_Sableantelope",
    "Permit_Saltwatercrocodile",
    "Permit_Siberiantiger",
    "Permit_Snowleopard",
    "Permit_Spottedhyena",
    "Permit_Springbok",
    "Permit_Timberwolf",
    "Permit_Titanbeetle",
    "Permit_WestAfricanlion",
    "Permit_Westernchimpanzee",
    "Permit_Westerndiamondbackrattlesnake",
    "Permit_Westernlowlandgorilla",
    "Permit_Yellowanaconda"
}

-- Number of permits currently owned (any species). Milestone rules use "$has_permits|N" =
-- rules.py HasGroup('Permits', N).
function count_Permits()
    local n = 0
    for _, code in ipairs(PERMIT_CODES) do
        if Tracker:ProviderCountForCode(code) > 0 then
            n = n + 1
        end
    end
    return n
end

function has_permits(n)
    return count_Permits() >= tonumber(n)
end
