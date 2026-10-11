from __future__ import annotations
from collections import namedtuple
from enum import Enum
import json
import pkgutil
import re
from typing import TYPE_CHECKING, List
from dataclasses import dataclass, replace
from BaseClasses import Item, ItemClassification
from .names import ItemNames

if TYPE_CHECKING:
    from .world import PlanetZooWorld


@dataclass
class ItemData:
    name: str
    ap_classification: ItemClassification
    version: str
    quantity: int
    id: int

    def __init__(self, name, ap_classification, version, quantity, id):
        self.name = name
        self.ap_classification = convert_ap_class(ap_classification)
        self.version = version
        self.quantity = quantity
        self.id = id


def convert_ap_class(ap_string):
    # Create list of Items
    ap_class = ItemClassification.useful
    # print(item)
    match ap_string:
        case "Filler":
            ap_class = ItemClassification.filler
        case "Progression":
            ap_class = ItemClassification.progression
        case "Useful":
            ap_class = ItemClassification.useful
        case "Trap":
            ap_class = ItemClassification.trap
        case "SkipBalancing":
            ap_class = ItemClassification.skip_balancing
        case "ProgressionSkipBalancing":
            ap_class = ItemClassification.progression_skip_balancing
        case "Deprioritized":
            ap_class = ItemClassification.deprioritized
    return ap_class


def item_decoder(objdict):
    return ItemData(**objdict)


def get_item_by_name(name: str) -> ItemData | None:
    return next(
        (item for item in complete_item_list if item.name == name),
        None
    )


complete_item_list: List[ItemData] = []


raw_data = pkgutil.get_data(
    __name__, "data/items.json")
if raw_data:
    complete_item_list = json.loads(raw_data, object_hook=item_decoder)


class Interactivity(str, Enum):
    full = "full"
    exhibit = "exhibit"


@dataclass
class SpeciesesFormation:
    def __init__(self, stringid, label, interactivity, water_needed, fence_grade):
        self.stringid = stringid
        self.label = label
        self.interactivity = interactivity
        self.water_needed = water_needed
        self.fence_grade = fence_grade
    # stringid: str
    # label: str
    # interactivity: Interactivity
    # water_needed: bool
    # fence_grade: int


def object_decoder(obj):
    return SpeciesesFormation(obj['stringid'], obj['label'], obj['interactivity'], obj['water_needed'], obj['fence_grade'])


def species_decoder(objdict):
    return namedtuple('SpeciesFormation', objdict.keys())(*objdict.values())


species_raw_data = pkgutil.get_data(
    __name__, "data/specieses.json")

if species_raw_data:
    species_data = json.loads(species_raw_data, object_hook=species_decoder)

species_data_list = list(species_data)


class PlanetZooItem(Item):
    game = "Planet Zoo"


def get_item_names_with_ids(item_names: list[ItemData]) -> dict[str, int]:
    result: dict[str, int] = {}
    for item_name in item_names:
        assert item_name.id is not None, f"{item_name.name} has no id assigned"
        result[item_name.name] = item_name.id
    return result


def create_item_with_correct_classification(world: PlanetZooWorld, name: str) -> PlanetZooItem:
    item = get_item_by_name(name)
    if item is None:  # avoid unassigned variable crashes
        raise ValueError(
            f"Item '{name}' not found - Check DLC filtering consitency")

    classification = item.ap_classification
    item_id = item.id
    return PlanetZooItem(name, classification, item_id, world.player)


def get_random_filler_item_name(world: PlanetZooWorld) -> str:
    roll = world.random.randint(0, 99)
    if roll < 5:
        return "Cash Inject Large"
    if roll < 25:
        return "Cash Inject Medium"
    if roll < 50:
        return "Cash Inject Small"
    if roll < 55:
        return "Conservation Credits Large"
    if roll < 75:
        return "Conservation Credits Medium"
    return "Conservation Credits Small"


# Permit list for Rule
list_of_permits = {
    item.name for item in complete_item_list if
    "Permit" in item.name
}


def permits_by_level(level: int) -> list[ItemData]:
    list_of_permits = [
        item for item in complete_item_list if
        "Permit" in item.name and "Giant Panda" not in item.name
    ]
    list_of_lvl_animals = [
        item for item in species_data_list if item.fence_grade == level and item.water_needed == False
    ]
    lvl_animal_names = {animal.label for animal in list_of_lvl_animals}
    permits_for_lvl_animals = [
        permit for permit in list_of_permits
        if permit.name.split(": ")[1] in lvl_animal_names
    ]
    return (permits_for_lvl_animals)


def create_starting_animals(world: PlanetZooWorld) -> List:
    permits_for_lv0_animals = permits_by_level(0)
    permits_for_lv1_animals = permits_by_level(1)
    permits_for_lv2_animals = permits_by_level(2)
    permits_for_lv3_animals = permits_by_level(3)
    permits_for_lv4_animals = permits_by_level(4)

    check_settings = []
    if world.options.barrier_lvl_start >= 0:
        check_settings = check_settings + permits_for_lv0_animals
    if world.options.barrier_lvl_start >= 1:
        check_settings = check_settings + permits_for_lv1_animals
    if world.options.barrier_lvl_start >= 2:
        check_settings = check_settings + permits_for_lv2_animals
    if world.options.barrier_lvl_start >= 3:
        check_settings = check_settings + permits_for_lv3_animals
    if world.options.barrier_lvl_start >= 4:
        check_settings = check_settings + permits_for_lv4_animals

    num_of_starting_species = int(world.options.num_starting_species)
    if num_of_starting_species > len(check_settings):
        num_of_starting_species = len(check_settings)

    starting_animals = world.random.sample(
        check_settings, num_of_starting_species)
    return (starting_animals)


def create_all_items(world: PlanetZooWorld) -> None:
    starting_animals = create_starting_animals(world)
    # starting_animals = sample(
    #     list_of_permits, world.options.num_starting_species)
    starting_animals_names = {item.name for item in starting_animals}
    reduced_item_list = [replace(item, quantity=item.quantity - world.options.barrier_lvl_start) if item.name ==
                         "Progressive Barrier Level" else item for item in complete_item_list if item.name not in starting_animals_names]
    # reduced_item_list = [
    #     item for item in complete_item_list if item.name not in starting_animals_names]
    if world.options.barrier_lvl_start > 0:
        # progressive_barrier = next(
        #     item for item in reduced_item_list if item.name == "Progressive Barrier Level")
        # progressive_barrier.quantity -= world.options.barrier_lvl_start
        for _ in range(world.options.barrier_lvl_start):
            starting_animals.append(
                ItemData(
                    "Progressive Barrier Level",
                    ItemClassification.progression,
                    "Base",
                    1,
                    12001))

    for item in starting_animals:
        world.multiworld.push_precollected(
            create_item_with_correct_classification(world, item.name))
    for item in reduced_item_list:
        world.multiworld.itempool.append(
            create_item_with_correct_classification(world, item.name))
        if item.quantity > 1:
            for _ in range(item.quantity-1):
                world.multiworld.itempool.append(
                    create_item_with_correct_classification(world, item.name))

    number_of_items = len(
        [item for item in world.multiworld.itempool if item.player == world.player])
    number_of_unfilled_locations = len(
        world.multiworld.get_unfilled_locations(world.player))
    # print(world.multiworld.get_unfilled_locations(world.player))  # Test
    needed_number_of_filler_items = number_of_unfilled_locations - \
        number_of_items-1  # Takes the goal location out of the location choices
    # Can filter out the goal location, instead of doing -1 (fix for later)
    world.multiworld.itempool += [world.create_filler()
                                  for _ in range(needed_number_of_filler_items)]
