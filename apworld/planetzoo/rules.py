from __future__ import annotations
from functools import reduce
from typing import TYPE_CHECKING, List
from rule_builder.rules import Has, HasAllCounts, HasGroup
from .items import ItemNames
from .names import EntranceNames
from .locations import LocationFormation, complete_location_list
if TYPE_CHECKING:
    from .world import PlanetZooWorld


HAS_CENTRE = Has(ItemNames.research_centre)
HAS_WORKSHOP = Has(ItemNames.workshop)
HAS_CONS_PROG = Has(ItemNames.conserve_program)
HAS_WATER_TOOLS = Has(ItemNames.water_hab_tools)
guest_counts_to_species = {"250": [0, 0],
                           "500": [6, 2],
                           "750": [8, 2],
                           "1000": [10, 3],
                           "1250": [12, 3],
                           "2500": [18, 4]}

zoo_rating_to_species = {"1": [0, 0],
                         "2": [4, 1],
                         "3": [6, 1],
                         "4": [8, 2],
                         "5": [18, 3]}


def set_all_rules(world: PlanetZooWorld) -> None:
    set_all_entrance_rules(world)
    set_all_location_rules(world)
    set_completion_condition(world)

# Research and Mechanics require buildings to be accessible


def set_all_entrance_rules(world: PlanetZooWorld) -> None:
    menu_to_research = world.multiworld.get_entrance(
        EntranceNames.menu_to_research, world.player)
    world.set_rule(menu_to_research, HAS_CENTRE)

    menu_to_mechanic = world.multiworld.get_entrance(
        EntranceNames.menu_to_mechanic, world.player)
    world.set_rule(menu_to_mechanic, HAS_WORKSHOP)
 # Need to release at least one animal to unlock Conservation Program
    cons_program = world.multiworld.get_entrance(
        EntranceNames.menu_to_conservation, world.player)
    world.set_rule(cons_program, HAS_CONS_PROG)


def set_species_gate(location: LocationFormation) -> List:
    condition = []
    animal_name = location.label.split(" - ")[0]
    animal_permit = f"Permit: {animal_name}"
    condition.append(Has(animal_permit))
    if location.water_needed == True:
        condition.append(HAS_WATER_TOOLS)
        # Water check rule
    if location.fence_grade > 0:
        condition.append(Has(
            "Progressive Barrier Level", location.fence_grade))
    return condition


# Logic graph integrated here?
def set_all_location_rules(world: PlanetZooWorld) -> None:
    # Animals that area gated rules are here
    # Permits for seperate animals

    for location in complete_location_list:
        condition = []
        if location.stringid == "fb_gpanda":
            continue
        if location.species_type != "none":
            condition = condition + set_species_gate(location)
        if "guests" in location.stringid:
            guest_count = location.label.split(" - ")[1]
            condition.append(
                HasGroup('Permits', guest_counts_to_species[guest_count][0])
            )
            condition.append(
                Has('Progressive Barrier Level',
                    guest_counts_to_species[guest_count][1])
            )
        if "rating" in location.stringid:
            rating_level = location.label.split(" - ")[1]
            condition.append(
                HasGroup('Permits', zoo_rating_to_species[rating_level][0])
            )
            condition.append(
                Has('Progressive Barrier Level',
                    zoo_rating_to_species[rating_level][1])
            )
        if condition:
            current_location = world.multiworld.get_location(
                location.label, world.player)
            combined_rule = reduce(lambda a, b: a & b, condition)
            world.set_rule(current_location, combined_rule)


# Goal currently is, breed Giant Pandas
def set_completion_condition(world: PlanetZooWorld) -> None:
    goal_location = world.multiworld.get_location("Giant Panda - First Breeding",
                                                  world.player)
    goal_location.place_locked_item(world.create_event("Victory"))
    world.set_rule(goal_location, HasAllCounts(
        {"Permit: Giant Panda": 1, "Progressive Barrier Level": 4}))
    world.multiworld.completion_condition[world.player] = lambda state: state.has(
        "Victory", world.player)
