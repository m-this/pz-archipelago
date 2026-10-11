from . import PlanetZooTestBase


class TestDefaultOptions(PlanetZooTestBase):
    pass


class TestEverySpeciesAtStart(PlanetZooTestBase):
    options = {"num_starting_species": 78}


class TestNoBarrierAtStart(PlanetZooTestBase):
    options = {"barrier_lvl_start": 0, "num_starting_species": 1}


class TestEveryBarrierAtStart(PlanetZooTestBase):
    options = {"barrier_lvl_start": 6}
